package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	logger *Logger
	cfg    *Config

	runMu    sync.Mutex
	pauseMu  sync.Mutex
	isPaused bool
	lastKill int
)

func appDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "MemGuard")
}

func main() {
	dryRun := false
	once := false
	for _, a := range os.Args[1:] {
		switch a {
		case "--dry-run":
			dryRun = true
		case "--once":
			once = true
		}
	}

	base := appDir()
	cfg = LoadConfig(base)
	if dryRun {
		cfg.DryRun = true
	}
	lg, err := NewLogger(filepath.Join(base, "logs"))
	if err != nil {
		return
	}
	logger = lg
	defer logger.Close()

	if once {
		res := RunRound(cfg)
		WriteLastRun(base, res)
		fmt.Printf("%s\n%s", res.summary(), mustRead(filepath.Join(base, "last-run.txt")))
		return
	}

	if !acquireSingleInstance() {
		return
	}

	icoPath := ensureIconFile(base)
	logf("MemGuard 启动，配置: interval=%dmin delay=%ds dryRun=%v", cfg.IntervalMinutes, cfg.StartupDelaySeconds, cfg.DryRun)
	runTray(cfg, base, icoPath)
	logf("MemGuard 退出")
}

func mustRead(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// acquireSingleInstance 用命名互斥量保证只有一个实例
func acquireSingleInstance() bool {
	name := utf16Ptr(`Local\MemGuardSingleInstance`)
	_, _, e := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if e == syscall.ERROR_ALREADY_EXISTS {
		return false
	}
	runtime.KeepAlive(name)
	return true
}

// ensureIconFile 把内嵌图标落到磁盘，供托盘 LoadImageW 使用
func ensureIconFile(base string) string {
	if err := os.MkdirAll(base, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(base, "memguard.ico")
	if st, err := os.Stat(path); err == nil && st.Size() == int64(len(embeddedIcon)) {
		return path
	}
	if err := os.WriteFile(path, embeddedIcon, 0o644); err != nil {
		return ""
	}
	return path
}

func paused() bool {
	pauseMu.Lock()
	defer pauseMu.Unlock()
	return isPaused
}

func togglePaused() bool {
	pauseMu.Lock()
	defer pauseMu.Unlock()
	isPaused = !isPaused
	return isPaused
}

func setLastKill(n int) {
	pauseMu.Lock()
	defer pauseMu.Unlock()
	lastKill = n
}

func getLastKill() int {
	pauseMu.Lock()
	defer pauseMu.Unlock()
	return lastKill
}

func toUTF16(dst []uint16, s string) {
	src := utf16.Encode([]rune(s))
	n := len(dst) - 1
	if len(src) < n {
		n = len(src)
	}
	copy(dst[:n], src[:n])
	dst[n] = 0
}
