package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	menuClean  = 1001
	menuPause  = 1002
	menuLogs   = 1003
	menuStatus = 1004
	menuExit   = 1005
	timerID    = 1
)

var (
	trayHWnd syscall.Handle
	trayMu   sync.Mutex
)

func runTray(c *Config, base string, icoPath string) {
	// 窗口必须在创建它的线程上收消息，否则 SetTimer 的消息不会被投递
	runtime.LockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className := utf16Ptr("MemGuardTrayWindow")
	title := utf16Ptr("MemGuard")

	wc := wndClassEx{
		HInstance:     syscall.Handle(hInst),
		LpszClassName: className,
		LpfnWndProc:   syscall.NewCallback(wndProc),
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	if r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		logf("RegisterClassExW 失败")
		return
	}

	hwnd, _, _ := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		0, 0, 0, 0, 0, 0, 0, hInst, 0)
	if hwnd == 0 {
		logf("CreateWindowExW 失败")
		return
	}
	trayHWnd = syscall.Handle(hwnd)

	ok := addTrayIcon(icoPath)
	logf("托盘窗口已创建，图标注册=%v，首轮将在 %ds 后执行", ok, c.StartupDelaySeconds)

	delay := c.StartupDelaySeconds
	if delay <= 0 {
		delay = 60
	}
	procSetTimer.Call(hwnd, timerID, uintptr(delay*1000), 0)

	messageLoop()
}

func messageLoop() {
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd, msgID, wparam, lparam uintptr) uintptr {
	switch uint32(msgID) {
	case wmTimer:
		if uint32(wparam) == timerID {
			scheduleNext()
			runRoundAsync(false)
		}
		return 0
	case wmCommand:
		switch loword(wparam) {
		case menuClean:
			runRoundAsync(true)
		case menuPause:
			if togglePaused() {
				balloon("MemGuard", "已暂停自动清理")
				logf("用户暂停自动清理")
			} else {
				balloon("MemGuard", "已恢复自动清理")
				logf("用户恢复自动清理")
			}
		case menuLogs:
			dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "MemGuard", "logs")
			openFolder(dir)
		case menuStatus:
			showStatus()
		case menuExit:
			exitApp(hwnd)
		}
		return 0
	case trayMsg:
		switch uint32(lparam) {
		case 0x0205: // WM_RBUTTONUP
			showMenu()
		case 0x0202: // WM_LBUTTONUP
			showStatus()
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wparam, lparam)
	return r
}

func scheduleNext() {
	interval := cfg.IntervalMinutes
	if interval <= 0 {
		interval = 10
	}
	procKillTimer.Call(uintptr(trayHWnd), timerID)
	procSetTimer.Call(uintptr(trayHWnd), timerID, uintptr(interval*60*1000), 0)
}

func runRoundAsync(manual bool) {
	if paused() && !manual {
		return
	}
	if !runMu.TryLock() {
		if manual {
			balloon("MemGuard", "上一轮仍在执行")
		}
		return
	}
	go func() {
		defer runMu.Unlock()
		res := RunRound(cfg)
		setLastKill(len(res.Killed))
		WriteLastRun(filepath.Join(os.Getenv("LOCALAPPDATA"), "MemGuard"), res)
		balloon("MemGuard", res.summary())
	}()
}

func addTrayIcon(icoPath string) bool {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayHWnd
	nid.UID = 1
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = trayMsg
	nid.HIcon = loadIcon(icoPath)
	toUTF16(nid.SzTip[:], "MemGuard 进程清理")
	r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	return r != 0
}

func loadIcon(icoPath string) syscall.Handle {
	if icoPath != "" {
		p := utf16Ptr(icoPath)
		if p != nil {
			h, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(p)), imageIcon, 0, 0, lrLoadFromFile|lrDefaultSize)
			if h != 0 {
				return syscall.Handle(h)
			}
		}
	}
	h, _, _ := procLoadIconW.Call(0, idiApplication)
	return syscall.Handle(h)
}

func balloon(title, text string) {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayHWnd
	nid.UID = 1
	nid.UFlags = nifInfo
	toUTF16(nid.SzInfoTitle[:], title)
	toUTF16(nid.SzInfo[:], text)
	nid.DwInfoFlags = niifInfo
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

func showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	pauseFlags := uintptr(mfString)
	pauseText := "暂停自动清理"
	if paused() {
		pauseFlags |= mfChecked
		pauseText = "恢复自动清理"
	}
	appendItem(menu, mfString, menuClean, "立即清理")
	appendItem(menu, pauseFlags, menuPause, pauseText)
	appendItem(menu, mfSeparator, 0, "")
	appendItem(menu, mfString, menuStatus, "当前状态")
	appendItem(menu, mfString, menuLogs, "打开日志目录")
	appendItem(menu, mfSeparator, 0, "")
	appendItem(menu, mfString, menuExit, "退出")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(trayHWnd))
	procTrackPopupMenu.Call(menu, tpmRightButton|tpmLeftAlign,
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(trayHWnd), 0)
	procPostMessageW.Call(uintptr(trayHWnd), wmNull, 0, 0)
	procDestroyMenu.Call(menu)
}

func appendItem(menu, flags, id uintptr, text string) {
	var p uintptr
	if text != "" {
		p = uintptr(unsafe.Pointer(utf16Ptr(text)))
	}
	procAppendMenuW.Call(menu, flags, id, p)
}

func showStatus() {
	e, c, n := StatusCounts()
	text := "explorer " + itoa(e) + " 个 / conhost " + itoa(c) + " 个 / node " + itoa(n) + " 个\n上轮结束: " + itoa(getLastKill()) + " 个"
	balloon("MemGuard 当前状态", text)
}

func openFolder(dir string) {
	_ = os.MkdirAll(dir, 0o755)
	openPath(dir)
}

func exitApp(hwnd uintptr) {
	var nid notifyIconData
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayHWnd
	nid.UID = 1
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	procPostQuitMessage.Call(0)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
