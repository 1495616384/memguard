package main

import (
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ProcInfo 是进程快照中的一条记录
type ProcInfo struct {
	PID      uint32
	PPID     uint32
	Name     string // 小写，不含 .exe
	MemBytes uint64
	Created  time.Time
	Cmdline  string
}

func (p *ProcInfo) MemMB() int { return int(p.MemBytes / (1024 * 1024)) }

// Snapshot 是所有进程的只读快照
type Snapshot struct {
	List []*ProcInfo
	ByID map[uint32]*ProcInfo
}

// TakeSnapshot 用 Toolhelp 快照枚举进程，避免 WMI 的启动开销
func TakeSnapshot() (*Snapshot, error) {
	h, _, err := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if h == invalidHandle {
		return nil, err
	}
	defer procCloseHandle.Call(h)

	s := &Snapshot{ByID: make(map[uint32]*ProcInfo)}
	var pe processEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	ret, _, err := procProcess32FirstW.Call(h, uintptr(unsafe.Pointer(&pe)))
	if ret == 0 {
		return nil, err
	}
	for {
		p := &ProcInfo{
			PID:  pe.ProcessID,
			PPID: pe.ParentProcessID,
			Name: strings.TrimSuffix(strings.ToLower(syscall.UTF16ToString(pe.ExeFile[:])), ".exe"),
		}
		fillMemory(p)
		fillCreateTime(p)
		fillCmdline(p)
		s.List = append(s.List, p)
		s.ByID[p.PID] = p

		pe.Size = uint32(unsafe.Sizeof(pe))
		if ok, _, _ := procProcess32NextW.Call(h, uintptr(unsafe.Pointer(&pe))); ok == 0 {
			break
		}
	}
	return s, nil
}

func openProcess(pid uint32, access uint32) (syscall.Handle, bool) {
	h, _, _ := procOpenProcess.Call(uintptr(access), 0, uintptr(pid))
	if h == 0 {
		return 0, false
	}
	return syscall.Handle(h), true
}

func fillMemory(p *ProcInfo) {
	h, ok := openProcess(p.PID, processQueryLimitedInfo|processVMRead)
	if !ok {
		return
	}
	defer procCloseHandle.Call(uintptr(h))

	var mc processMemoryCounters
	mc.CB = uint32(unsafe.Sizeof(mc))
	if r, _, _ := procK32GetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.CB)); r != 0 {
		p.MemBytes = uint64(mc.WorkingSetSize)
	}
}

func fillCreateTime(p *ProcInfo) {
	h, ok := openProcess(p.PID, processQueryLimitedInfo)
	if !ok {
		return
	}
	defer procCloseHandle.Call(uintptr(h))

	var c, e, k, u filetime
	if r, _, _ := procGetProcessTimes.Call(uintptr(h), uintptr(unsafe.Pointer(&c)),
		uintptr(unsafe.Pointer(&e)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u))); r != 0 {
		ft := syscall.Filetime{LowDateTime: c.LowDateTime, HighDateTime: c.HighDateTime}
		p.Created = time.Unix(0, ft.Nanoseconds())
	}
}

// fillCmdline 读取目标进程 PEB 中的命令行；失败时留空，调用方必须按“未知”处理
func fillCmdline(p *ProcInfo) {
	h, ok := openProcess(p.PID, processQueryLimitedInfo|processVMRead)
	if !ok {
		return
	}
	defer procCloseHandle.Call(uintptr(h))

	var pbi processBasicInformation
	var retLen uint32
	r, _, _ := procNtQueryInformationProcess.Call(uintptr(h), 0,
		uintptr(unsafe.Pointer(&pbi)), uintptr(unsafe.Sizeof(pbi)), uintptr(unsafe.Pointer(&retLen)))
	if int32(r) < 0 || pbi.PebBaseAddress == 0 {
		return
	}

	var params uintptr
	if !readMem(h, pbi.PebBaseAddress+0x20, unsafe.Pointer(&params), unsafe.Sizeof(params)) {
		return
	}
	var us unicodeString
	if !readMem(h, params+0x70, unsafe.Pointer(&us), unsafe.Sizeof(us)) {
		return
	}
	if us.Length == 0 || us.Length > 8192 || us.Buffer == 0 {
		return
	}
	buf := make([]uint16, us.Length/2)
	if !readMem(h, us.Buffer, unsafe.Pointer(&buf[0]), uintptr(len(buf))*2) {
		return
	}
	p.Cmdline = string(utf16Decode(buf))
}

func readMem(h syscall.Handle, addr uintptr, dst unsafe.Pointer, size uintptr) bool {
	var read uintptr
	r, _, _ := procReadProcessMemory.Call(uintptr(h), addr, uintptr(dst), size, uintptr(unsafe.Pointer(&read)))
	return r != 0 && read == size
}

// HasVisibleWindow 判断进程是否拥有可见的顶层窗口
func HasVisibleWindow(pid uint32) bool {
	found := false
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if found {
			return 1
		}
		var wpid uint32
		procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if wpid != pid {
			return 1
		}
		if vis, _, _ := procIsWindowVisible.Call(hwnd); vis == 0 {
			return 1
		}
		owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
		if owner != 0 {
			return 1
		}
		found = true
		return 0
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// AncestorNames 向上追溯进程链，返回沿途的进程名（最多 10 层）
func (s *Snapshot) AncestorNames(pid uint32) []string {
	var names []string
	seen := map[uint32]bool{}
	cur := pid
	for i := 0; i < 10; i++ {
		p, ok := s.ByID[cur]
		if !ok || seen[p.PPID] {
			break
		}
		seen[p.PPID] = true
		parent, ok := s.ByID[p.PPID]
		if !ok {
			break
		}
		names = append(names, parent.Name)
		cur = parent.PPID
	}
	return names
}

// Kill 结束进程
func Kill(pid uint32) bool {
	h, ok := openProcess(pid, processTerminate)
	if !ok {
		return false
	}
	defer procCloseHandle.Call(uintptr(h))
	r, _, _ := procTerminateProcess.Call(uintptr(h), 1)
	return r != 0
}
