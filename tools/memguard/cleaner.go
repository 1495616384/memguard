package main

import (
	"os"
	"strings"
	"sync"
	"time"
)

// hardBlocked 永远不允许结束的进程名
var hardBlocked = map[string]bool{
	"system": true, "registry": true, "idle": true, "memory compression": true,
	"wininit": true, "winlogon": true, "services": true, "csrss": true,
	"lsass": true, "smss": true, "dwm": true, "svchost": true,
	"msmpeng": true, "nissrv": true, "hipsdaemon": true, "hipstray": true,
	"securityhealthservice": true, "memguard": true,
}

// ideAncestors 视为“仍在被使用”的祖先进程
var ideAncestors = map[string]bool{
	"trae cn": true, "workbuddy": true, "cmd": true, "pwsh": true,
	"powershell": true, "windowsterminal": true, "wt": true,
}

const minOrphanAgeMinutes = 5

// failedCooldown 记录结束失败的 PID，30 分钟内不再重复尝试，避免日志刷屏
var (
	failedMu   sync.Mutex
	failedSeen = map[uint32]time.Time{}
)

const failedCooldown = 30 * time.Minute

func inCooldown(pid uint32) bool {
	failedMu.Lock()
	defer failedMu.Unlock()
	t, ok := failedSeen[pid]
	if !ok {
		return false
	}
	if time.Since(t) > failedCooldown {
		delete(failedSeen, pid)
		return false
	}
	return true
}

func markFailed(pid uint32) {
	failedMu.Lock()
	failedSeen[pid] = time.Now()
	failedMu.Unlock()
}

// RunRound 执行一轮保守清理
func RunRound(cfg *Config) RoundResult {
	res := RoundResult{Start: time.Now(), DryRun: cfg.DryRun}

	snap, err := TakeSnapshot()
	if err != nil {
		res.Note = "进程快照失败，整轮跳过: " + err.Error()
		logf("%s", res.Note)
		return res
	}
	res.Scanned = len(snap.List)

	selfPID := uint32(os.Getpid())
	var selfPPID uint32
	if sp, ok := snap.ByID[selfPID]; ok {
		selfPPID = sp.PPID
	}

	budget := cfg.MaxKillPerRound
	handled := map[uint32]bool{}

	doKill := func(rule string, p *ProcInfo, reason string) {
		if handled[p.PID] || p.PID == selfPID || p.PID == selfPPID {
			return
		}
		if hardBlocked[p.Name] {
			logf("%-16s 跳过受保护进程 %s(PID=%d)", rule, p.Name, p.PID)
			return
		}
		if inCooldown(p.PID) {
			return
		}
		if budget <= 0 {
			res.Skipped++
			logf("%-16s 超出本轮上限，跳过 %s(PID=%d)", rule, p.Name, p.PID)
			return
		}
		handled[p.PID] = true
		budget--

		rec := KillRecord{Rule: rule, PID: p.PID, Name: p.Name, MemMB: p.MemMB(), Reason: reason}
		if cfg.DryRun {
			res.Killed = append(res.Killed, rec)
			logf("%-16s DRY-RUN 命中 %s(PID=%d) %dMB %s", rule, p.Name, p.PID, rec.MemMB, reason)
			return
		}
		if !Kill(p.PID) {
			markFailed(p.PID)
			logf("%-16s 结束失败 %s(PID=%d) %dMB，%d 分钟内不再重试", rule, p.Name, p.PID, rec.MemMB, int(failedCooldown.Minutes()))
			return
		}
		res.Killed = append(res.Killed, rec)
		logf("%-16s 已结束 %s(PID=%d) %dMB %s", rule, p.Name, p.PID, rec.MemMB, reason)
	}

	if cfg.Enable.Explorer {
		cleanZombieExplorer(snap, cfg, doKill)
	}
	if cfg.Enable.Conhost {
		cleanOrphan(snap, "conhost", "orphan-conhost", doKill)
	}
	if cfg.Enable.OrphanNode {
		cleanOrphanNode(snap, doKill)
	}
	if cfg.Enable.DuplicateMcp {
		cleanDuplicateMCP(snap, doKill)
	}

	logf("本轮完成: %s", res.summary())
	return res
}

// R1 僵尸 explorer：仅当实例数 > 1、内存低于阈值、且无可见窗口
func cleanZombieExplorer(snap *Snapshot, cfg *Config, doKill func(string, *ProcInfo, string)) {
	var exps []*ProcInfo
	for _, p := range snap.List {
		if p.Name == "explorer" {
			exps = append(exps, p)
		}
	}
	if len(exps) < 2 {
		return
	}
	keeper := exps[0]
	for _, p := range exps[1:] {
		if p.MemBytes > keeper.MemBytes {
			keeper = p
		}
	}
	for _, p := range exps {
		if p.PID == keeper.PID {
			continue
		}
		if p.MemMB() >= cfg.ZombieExplorerMaxMB {
			continue
		}
		if HasVisibleWindow(p.PID) {
			continue
		}
		doKill("zombie-explorer", p, "内存低于阈值且无可见窗口")
	}
}

// R2 孤儿 conhost
func cleanOrphan(snap *Snapshot, name, rule string, doKill func(string, *ProcInfo, string)) {
	for _, p := range snap.List {
		if p.Name != name {
			continue
		}
		if _, ok := snap.ByID[p.PPID]; ok {
			continue
		}
		doKill(rule, p, "父进程已退出")
	}
}

// R3 孤儿 node：父进程不存在且无存活的 IDE/终端祖辈
func cleanOrphanNode(snap *Snapshot, doKill func(string, *ProcInfo, string)) {
	for _, p := range snap.List {
		if p.Name != "node" {
			continue
		}
		if _, ok := snap.ByID[p.PPID]; ok {
			continue
		}
		if p.Created.IsZero() || time.Since(p.Created) < minOrphanAgeMinutes*time.Minute {
			continue
		}
		if hasLiveIDEAncestor(snap, p.PID) {
			continue
		}
		doKill("orphan-node", p, "父进程已退出且无存活祖辈")
	}
}

// R4 重复 MCP 副本：同签名保留最新，其余仅在无存活 IDE 祖先时结束
func cleanDuplicateMCP(snap *Snapshot, doKill func(string, *ProcInfo, string)) {
	groups := map[string][]*ProcInfo{}
	for _, p := range snap.List {
		if p.Name != "node" {
			continue
		}
		sig := mcpSignature(p.Cmdline)
		if sig == "" {
			continue
		}
		groups[sig] = append(groups[sig], p)
	}
	for sig, list := range groups {
		if len(list) < 2 {
			continue
		}
		newest := list[0]
		for _, p := range list[1:] {
			if p.Created.After(newest.Created) {
				newest = p
			}
		}
		for _, p := range list {
			if p.PID == newest.PID {
				continue
			}
			if hasLiveIDEAncestor(snap, p.PID) {
				continue
			}
			doKill("duplicate-mcp", p, "重复副本且无存活 IDE 祖先: "+sig)
		}
	}
}

func hasLiveIDEAncestor(snap *Snapshot, pid uint32) bool {
	for _, n := range snap.AncestorNames(pid) {
		if ideAncestors[n] {
			return true
		}
	}
	return false
}

// mcpSignature 从命令行提取归一化的包签名，取不到时返回空串（调用方必须跳过）
func mcpSignature(cmd string) string {
	if cmd == "" {
		return ""
	}
	c := strings.ReplaceAll(cmd, "/", "\\")
	low := strings.ToLower(c)

	if i := strings.Index(low, "plugins\\cache\\workbuddy-builtin\\"); i >= 0 {
		parts := strings.Split(c[i+len("plugins\\cache\\workbuddy-builtin\\"):], "\\")
		if len(parts) > 0 && parts[0] != "" {
			return "wb/" + strings.ToLower(parts[0])
		}
	}

	if i := strings.Index(low, "node_modules\\"); i >= 0 {
		if seg := firstPackageSegment(c[i+len("node_modules\\"):]); seg != "" {
			return strings.ToLower(seg)
		}
	}

	if i := strings.Index(low, "npx-cli.js"); i >= 0 {
		for _, t := range strings.Fields(c[i+len("npx-cli.js"):]) {
			t = strings.Trim(t, "\"'")
			if t == "" || strings.HasPrefix(t, "-") {
				continue
			}
			return strings.ToLower(t)
		}
	}
	return ""
}

func firstPackageSegment(rest string) string {
	parts := strings.Split(rest, "\\")
	for i := 0; i < len(parts) && i < 6; i++ {
		seg := parts[i]
		if seg == "" || seg == "." || seg == ".." || seg == ".bin" {
			continue
		}
		if strings.HasPrefix(seg, "@") && i+1 < len(parts) {
			return seg + "/" + parts[i+1]
		}
		return seg
	}
	return ""
}

// StatusCounts 返回用于托盘展示的实例数
func StatusCounts() (explorer, conhost, node int) {
	snap, err := TakeSnapshot()
	if err != nil {
		return
	}
	for _, p := range snap.List {
		switch p.Name {
		case "explorer":
			explorer++
		case "conhost":
			conhost++
		case "node":
			node++
		}
	}
	return
}

func logf(format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}
