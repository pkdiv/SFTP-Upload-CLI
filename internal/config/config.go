package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const AppName = "uplift"

type Config struct {
	Profiles map[string]Profile `yaml:"profiles"`
}

type Profile struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Username   string `yaml:"username"`
	KeyFile    string `yaml:"key_file"`
	LocalBase  string `yaml:"local_base"`
	RemoteBase string `yaml:"remote_base"`
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine user config directory: %w", err)
	}
	return filepath.Join(base, AppName, "config.yaml"), nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Profiles: map[string]Profile{}}, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func ValidateProfileName(name string) error {
	if name == "" {
		return errors.New("profile name is required")
	}
	if !namePattern.MatchString(name) {
		return errors.New("profile name must contain only letters, numbers, dot, dash, or underscore and must not start with a dot or dash")
	}
	return nil
}

func ValidateProfile(p Profile) error {
	if strings.TrimSpace(p.Host) == "" {
		return errors.New("host is required")
	}
	if p.Port != 0 && (p.Port < 1 || p.Port > 65535) {
		return fmt.Errorf("port %d is invalid (must be 1-65535)", p.Port)
	}
	if strings.TrimSpace(p.Username) == "" {
		return errors.New("username is required")
	}
	if strings.TrimSpace(p.KeyFile) == "" {
		return errors.New("private key file is required")
	}
	if strings.TrimSpace(p.LocalBase) == "" {
		return errors.New("local base path is required")
	}
	if strings.TrimSpace(p.RemoteBase) == "" {
		return errors.New("remote base path is required")
	}
	return nil
}

func (p *Profile) Normalize() {
	p.Host = strings.TrimSpace(p.Host)
	p.Username = strings.TrimSpace(p.Username)
	p.KeyFile = expandHome(strings.TrimSpace(p.KeyFile))
	p.LocalBase = filepath.Clean(expandHome(strings.TrimSpace(p.LocalBase)))
	p.RemoteBase = cleanRemotePath(p.RemoteBase)
	if p.Port == 0 {
		p.Port = 22
	}
}

func cleanRemotePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	p = strings.ReplaceAll(p, "\\", "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimSuffix(p, "/")
}

func expandHome(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func (c *Config) Get(name string) (Profile, bool) {
	p, ok := c.Profiles[name]
	return p, ok
}

func (c *Config) Add(name string, p Profile) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if _, exists := c.Profiles[name]; exists {
		return fmt.Errorf("profile %q already exists", name)
	}
	if err := ValidateProfile(p); err != nil {
		return err
	}
	p.Normalize()
	c.Profiles[name] = p
	return nil
}

func (c *Config) Update(name string, p Profile) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if _, exists := c.Profiles[name]; !exists {
		return fmt.Errorf("profile %q does not exist", name)
	}
	if err := ValidateProfile(p); err != nil {
		return err
	}
	p.Normalize()
	c.Profiles[name] = p
	return nil
}

func (c *Config) Remove(name string) error {
	if _, exists := c.Profiles[name]; !exists {
		return fmt.Errorf("profile %q does not exist", name)
	}
	delete(c.Profiles, name)
	return nil
}

func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	return names
}


