package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Paths   PathsConfig   `yaml:"paths"`
	Workers WorkersConfig `yaml:"workers"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type PathsConfig struct {
	Claude    string `yaml:"claude"`
	Codex     string `yaml:"codex"`
	GitBash   string `yaml:"git_bash"`
	DataDir   string `yaml:"-"`
	DBPath    string `yaml:"-"`
	LogsDir   string `yaml:"-"`
	StaticDir string `yaml:"-"`
}

type WorkersConfig struct {
	// UnsafeAutomation enables "dangerously-*" flags for unattended runs.
	// Default is false to avoid autonomous approvals/actions.
	UnsafeAutomation bool `yaml:"unsafe_automation"`
}

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("config: cannot determine home dir: %w", err)
	}
	return filepath.Join(home, ".controlccx"), nil
}

func Default() Config {
	return Config{
		Server: ServerConfig{Addr: "127.0.0.1:5174"},
		Paths: PathsConfig{
			Claude:  "claude",
			Codex:   "codex",
			GitBash: defaultGitBashPath(),
		},
		Workers: WorkersConfig{
			UnsafeAutomation: false,
		},
	}
}

func Load(dataDir string) (Config, error) {
	cfg := Default()
	if dataDir == "" {
		var err error
		dataDir, err = DefaultDataDir()
		if err != nil {
			return Config{}, err
		}
	}
	dataDir = filepath.Clean(dataDir)

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return Config{}, fmt.Errorf("config: create data dir: %w", err)
	}

	path := filepath.Join(dataDir, "config.yaml")
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg.Paths.DataDir = dataDir
	cfg.Paths.DBPath = filepath.Join(dataDir, "controlccx.db")
	cfg.Paths.LogsDir = filepath.Join(dataDir, "logs")

	return cfg, nil
}

func defaultGitBashPath() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	// Best-effort common location; can be overridden by config.
	return `C:\Program Files\Git\bin\bash.exe`
}
