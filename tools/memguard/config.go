package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是托盘程序的全部可调参数，写在 %LOCALAPPDATA%\MemGuard\config.json
type Config struct {
	DryRun                 bool       `json:"dryRun"`
	IntervalMinutes        int        `json:"intervalMinutes"`
	StartupDelaySeconds    int        `json:"startupDelaySeconds"`
	MaxKillPerRound        int        `json:"maxKillPerRound"`
	ZombieExplorerMaxMB    int        `json:"zombieExplorerMaxMB"`
	Enable                 EnableConf `json:"enable"`
}

// EnableConf 控制四条保守规则各自是否启用
type EnableConf struct {
	Explorer     bool `json:"explorer"`
	Conhost      bool `json:"conhost"`
	OrphanNode   bool `json:"orphanNode"`
	DuplicateMcp bool `json:"duplicateMcp"`
}

func defaultConfig() *Config {
	return &Config{
		DryRun:              false,
		IntervalMinutes:     10,
		StartupDelaySeconds: 60,
		MaxKillPerRound:     20,
		ZombieExplorerMaxMB: 8,
		Enable: EnableConf{
			Explorer:     true,
			Conhost:      true,
			OrphanNode:   true,
			DuplicateMcp: true,
		},
	}
}

// LoadConfig 读取配置；文件不存在时写出默认值，损坏时回退默认值
func LoadConfig(baseDir string) *Config {
	path := filepath.Join(baseDir, "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		c := defaultConfig()
		c.Save(baseDir)
		return c
	}
	c := defaultConfig()
	if err := json.Unmarshal(data, c); err != nil {
		return defaultConfig()
	}
	if c.IntervalMinutes <= 0 {
		c.IntervalMinutes = 10
	}
	if c.MaxKillPerRound <= 0 {
		c.MaxKillPerRound = 20
	}
	if c.ZombieExplorerMaxMB <= 0 {
		c.ZombieExplorerMaxMB = 8
	}
	return c
}

// Save 将当前配置写回磁盘
func (c *Config) Save(baseDir string) error {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(baseDir, "config.json"), data, 0o644)
}
