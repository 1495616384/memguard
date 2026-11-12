package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Logger 是按天滚动的简易日志，保留 7 天
type Logger struct {
	dir  string
	mu   sync.Mutex
	day  string
	file *os.File
}

func NewLogger(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Logger{dir: dir}
	l.cleanup(7)
	return l, nil
}

func (l *Logger) cleanup(keepDays int) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	cut := time.Now().AddDate(0, 0, -keepDays)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "memguard-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cut) {
			_ = os.Remove(filepath.Join(l.dir, e.Name()))
		}
	}
}

func (l *Logger) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := time.Now().Format("20060102")
	if l.file == nil || l.day != day {
		if l.file != nil {
			_ = l.file.Close()
		}
		f, err := os.OpenFile(filepath.Join(l.dir, "memguard-"+day+".log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		l.file, l.day = f, day
	}
	_, _ = fmt.Fprintf(l.file, "%s | %s\n",
		time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, args...))
}

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// KillRecord 记录一次结束动作
type KillRecord struct {
	Rule   string
	PID    uint32
	Name   string
	MemMB  int
	Reason string
}

// RoundResult 是一轮清理的结果
type RoundResult struct {
	Start    time.Time
	DryRun   bool
	Scanned  int
	Killed   []KillRecord
	Skipped  int
	Note     string
}

func (r RoundResult) summary() string {
	if len(r.Killed) == 0 {
		return fmt.Sprintf("扫描 %d 个进程，未结束任何进程", r.Scanned)
	}
	return fmt.Sprintf("扫描 %d 个进程，结束 %d 个", r.Scanned, len(r.Killed))
}

// WriteLastRun 把最近一轮结果写成可读文本，供 --once 诊断使用
func WriteLastRun(baseDir string, r RoundResult) {
	var b strings.Builder
	mode := "正式"
	if r.DryRun {
		mode = "DRY-RUN"
	}
	fmt.Fprintf(&b, "时间: %s\n模式: %s\n结果: %s\n", r.Start.Format("2006-01-02 15:04:05"), mode, r.summary())
	if r.Note != "" {
		fmt.Fprintf(&b, "备注: %s\n", r.Note)
	}
	if len(r.Killed) > 0 {
		b.WriteString("\n明细:\n")
		recs := append([]KillRecord(nil), r.Killed...)
		sort.Slice(recs, func(i, j int) bool { return recs[i].Rule < recs[j].Rule })
		for _, k := range recs {
			fmt.Fprintf(&b, "  %-12s PID=%-7d %-12s %4dMB  %s\n", k.Rule, k.PID, k.Name, k.MemMB, k.Reason)
		}
	}
	_ = os.WriteFile(filepath.Join(baseDir, "last-run.txt"), []byte(b.String()), 0o644)
}
