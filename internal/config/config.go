package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type MountRoot struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Path        string `yaml:"path" json:"-"`
	ReadOnly    bool   `yaml:"read_only" json:"readOnly"`
	PrivateOnly bool   `yaml:"private_only" json:"privateOnly"`
}

type ServerConfig struct {
	PublicAddr    string `yaml:"public_addr"`
	PrivateAddr   string `yaml:"private_addr"`
	PublicBaseURL string `yaml:"public_base_url"`
	CookieSecure  bool   `yaml:"cookie_secure"`
}

type StorageConfig struct {
	DatabasePath  string      `yaml:"database_path"`
	TempUploadDir string      `yaml:"temp_upload_dir"`
	StacksDir     string      `yaml:"stacks_dir"`
	Roots         []MountRoot `yaml:"roots"`
}

type AuthConfig struct {
	InitialAdminUsername string `yaml:"initial_admin_username"`
	InitialAdminPassword string `yaml:"initial_admin_password"`
	SessionTTLHours      int    `yaml:"session_ttl_hours"`
}

type WargameConfig struct {
	BindIP          string `yaml:"bind_ip"`
	PortStart       int    `yaml:"port_start"`
	PortEnd         int    `yaml:"port_end"`
	WorkDir         string `yaml:"work_dir"`
	DefaultTTLHours int    `yaml:"default_ttl_hours"`
	MaxArchiveBytes int64  `yaml:"max_archive_bytes"`
}

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
	Auth    AuthConfig    `yaml:"auth"`
	Wargame WargameConfig `yaml:"wargame"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			PublicAddr:    "127.0.0.1:8080",
			PrivateAddr:   "127.0.0.1:8081",
			PublicBaseURL: "",
			CookieSecure:  false,
		},
		Storage: StorageConfig{
			DatabasePath:  "./data/personal-server.db",
			TempUploadDir: "./data/uploads",
			StacksDir:     "./data/stacks",
			Roots: []MountRoot{
				{ID: "media", Name: "Media (영상/미디어)", Path: "./data/roots/media"},
				{ID: "workspace", Name: "Workspace (작업 폴더)", Path: "./data/roots/workspace"},
			},
		},
		Auth: AuthConfig{
			InitialAdminUsername: "admin",
			InitialAdminPassword: "admin1234!",
			SessionTTLHours:      168,
		},
		Wargame: WargameConfig{
			BindIP:          "127.0.0.1",
			PortStart:       30000,
			PortEnd:         39999,
			WorkDir:         "./data/wargames",
			DefaultTTLHours: 3,
			MaxArchiveBytes: 1024 * 1024 * 1024,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return Config{}, fmt.Errorf("read config %s: %w", path, err)
			}
		} else if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	if cfg.Auth.SessionTTLHours <= 0 {
		cfg.Auth.SessionTTLHours = 168
	}
	if cfg.Wargame.BindIP == "" {
		cfg.Wargame.BindIP = "127.0.0.1"
	}
	if cfg.Wargame.PortStart <= 0 {
		cfg.Wargame.PortStart = 30000
	}
	if cfg.Wargame.PortEnd <= cfg.Wargame.PortStart {
		cfg.Wargame.PortEnd = cfg.Wargame.PortStart + 9999
	}
	if cfg.Wargame.DefaultTTLHours <= 0 {
		cfg.Wargame.DefaultTTLHours = 3
	}
	if cfg.Wargame.MaxArchiveBytes <= 0 {
		cfg.Wargame.MaxArchiveBytes = 1024 * 1024 * 1024
	}
	if cfg.Wargame.WorkDir == "" {
		cfg.Wargame.WorkDir = "./data/wargames"
	}
	if err := os.MkdirAll(cfg.Wargame.WorkDir, 0o755); err != nil {
		return Config{}, fmt.Errorf("create wargame dir: %w", err)
	}
	if wgAbs, err := filepath.Abs(cfg.Wargame.WorkDir); err == nil {
		if wgReal, evalErr := filepath.EvalSymlinks(wgAbs); evalErr == nil {
			cfg.Wargame.WorkDir = wgReal
		}
	}
	if len(cfg.Storage.Roots) == 0 {
		return Config{}, errors.New("storage.roots must contain at least one directory")
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Storage.DatabasePath), 0o755); err != nil {
		return Config{}, fmt.Errorf("create db dir: %w", err)
	}
	if err := os.MkdirAll(cfg.Storage.TempUploadDir, 0o755); err != nil {
		return Config{}, fmt.Errorf("create upload dir: %w", err)
	}
	if cfg.Storage.StacksDir == "" {
		cfg.Storage.StacksDir = "./data/stacks"
	}
	if err := os.MkdirAll(cfg.Storage.StacksDir, 0o755); err != nil {
		return Config{}, fmt.Errorf("create stacks dir: %w", err)
	}
	if stAbs, err := filepath.Abs(cfg.Storage.StacksDir); err == nil {
		if stReal, evalErr := filepath.EvalSymlinks(stAbs); evalErr == nil {
			cfg.Storage.StacksDir = stReal
		}
	}

	seen := make(map[string]struct{}, len(cfg.Storage.Roots))
	for i := range cfg.Storage.Roots {
		r := &cfg.Storage.Roots[i]
		if r.ID == "" {
			return Config{}, errors.New("each storage root requires a non-empty id")
		}
		if _, exists := seen[r.ID]; exists {
			return Config{}, fmt.Errorf("duplicate storage root id %q", r.ID)
		}
		seen[r.ID] = struct{}{}

		if err := os.MkdirAll(r.Path, 0o755); err != nil {
			return Config{}, fmt.Errorf("ensure root %s (%s): %w", r.ID, r.Path, err)
		}
		abs, err := filepath.Abs(r.Path)
		if err != nil {
			return Config{}, fmt.Errorf("abs root %s: %w", r.ID, err)
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return Config{}, fmt.Errorf("eval symlinks root %s: %w", r.ID, err)
		}
		r.Path = real
	}

	return cfg, nil
}
