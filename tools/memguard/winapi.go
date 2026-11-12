package main

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modNtdll    = syscall.NewLazyDLL("ntdll.dll")

	procCreateToolhelp32Snapshot = modKernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = modKernel32.NewProc("Process32FirstW")
	procProcess32NextW           = modKernel32.NewProc("Process32NextW")
	procOpenProcess              = modKernel32.NewProc("OpenProcess")
	procCloseHandle              = modKernel32.NewProc("CloseHandle")
	procTerminateProcess         = modKernel32.NewProc("TerminateProcess")
	procGetProcessTimes          = modKernel32.NewProc("GetProcessTimes")
	procK32GetProcessMemoryInfo  = modKernel32.NewProc("K32GetProcessMemoryInfo")
	procCreateMutexW             = modKernel32.NewProc("CreateMutexW")
	procReadProcessMemory        = modKernel32.NewProc("ReadProcessMemory")
	procGetModuleHandleW         = modKernel32.NewProc("GetModuleHandleW")

	procNtQueryInformationProcess = modNtdll.NewProc("NtQueryInformationProcess")

	procRegisterClassExW  = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW   = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW    = modUser32.NewProc("DefWindowProcW")
	procGetMessageW       = modUser32.NewProc("GetMessageW")
	procTranslateMessage  = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW  = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage   = modUser32.NewProc("PostQuitMessage")
	procCreatePopupMenu   = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW       = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu    = modUser32.NewProc("TrackPopupMenu")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procPostMessageW      = modUser32.NewProc("PostMessageW")
	procGetCursorPos      = modUser32.NewProc("GetCursorPos")
	procSetTimer          = modUser32.NewProc("SetTimer")
	procKillTimer         = modUser32.NewProc("KillTimer")
	procLoadImageW        = modUser32.NewProc("LoadImageW")
	procLoadIconW         = modUser32.NewProc("LoadIconW")
	procDestroyMenu       = modUser32.NewProc("DestroyMenu")
	procEnumWindows       = modUser32.NewProc("EnumWindows")
	procIsWindowVisible   = modUser32.NewProc("IsWindowVisible")
	procGetWindow         = modUser32.NewProc("GetWindow")
	procGetWindowThreadProcessId = modUser32.NewProc("GetWindowThreadProcessId")
	procMessageBoxW       = modUser32.NewProc("MessageBoxW")

	procShellNotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	procShellExecuteW    = modShell32.NewProc("ShellExecuteW")
)

// openPath 用系统默认方式打开文件夹或文件
func openPath(target string) bool {
	op := utf16Ptr("open")
	tp := utf16Ptr(target)
	if op == nil || tp == nil {
		return false
	}
	r, _, _ := procShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(op)), uintptr(unsafe.Pointer(tp)), 0, 0, 1)
	return r > 32
}

const (
	th32csSnapProcess = 0x00000002
	invalidHandle     = ^uintptr(0)

	processTerminate            = 0x0001
	processQueryLimitedInfo     = 0x1000
	processVMRead               = 0x0010
	processQueryInformation     = 0x0400

	maxPath = 260

	wmDestroy = 0x0002
	wmCommand = 0x0111
	wmTimer   = 0x0113
	wmNull    = 0x0000
	trayMsg   = 0x8001 // WM_APP + 1

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	niifInfo = 0x00000001

	mfString       = 0x00000000
	mfSeparator    = 0x00000800
	mfGrayed       = 0x00000001
	mfChecked      = 0x00000008
	tpmRightButton = 0x0002
	tpmLeftAlign   = 0x0000

	imageIcon     = 1
	lrLoadFromFile = 0x00000010
	lrDefaultSize = 0x00000040

	idiApplication = 32512

	gwOwner = 4
)

type processEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [maxPath]uint16
}

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

type filetime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

type processBasicInformation struct {
	ExitStatus                   uintptr
	PebBaseAddress               uintptr
	AffinityMask                 uintptr
	BasePriority                 uintptr
	UniqueProcessID              uintptr
	InheritedFromUniqueProcessID uintptr
}

type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	_             uint32
	Buffer        uintptr
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         guid
	HBalloonIcon     syscall.Handle
}

func utf16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return nil
	}
	return p
}

func utf16ToString(buf []uint16) string {
	for i, v := range buf {
		if v == 0 {
			return string(utf16.Decode(buf[:i]))
		}
	}
	return string(utf16.Decode(buf))
}

func loword(v uintptr) uint32 { return uint32(v & 0xffff) }

func utf16Decode(buf []uint16) []rune { return utf16.Decode(buf) }
