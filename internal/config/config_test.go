package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := `profiles:
  production:
    host: example.com
    port: 22
    username: deploy
    key_file: ~/.ssh/id_ed25519
    local_base: /home/user/project
    remote_base: /var/www/app
`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cfg.Get("production")
	if !ok {
		t.Fatal("production profile not found")
	}
	if p.Host != "example.com" || p.Username != "deploy" || p.Port != 22 {
		t.Fatalf("unexpected profile: %+v", p)
	}
	if p.LocalBase != "/home/user/project" || p.RemoteBase != "/var/www/app" {
		t.Fatalf("unexpected base paths: %+v", p)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("profiles: [invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(cfg.Profiles))
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.yaml")
	cfg := &Config{Profiles: map[string]Profile{
		"staging": {
			Host:       "staging.example.com",
			Port:       2222,
			Username:   "deploy",
			KeyFile:    "~/.ssh/staging",
			LocalBase:  "/home/user/project",
			RemoteBase: "/var/www/staging",
		},
	}}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := loaded.Get("staging")
	if !ok {
		t.Fatal("staging profile missing after reload")
	}
	if p.Port != 2222 || p.Host != "staging.example.com" {
		t.Fatalf("unexpected profile: %+v", p)
	}
}

func TestValidateProfileMissingFields(t *testing.T) {
	cases := []struct {
		name    string
		profile Profile
	}{
		{"missing host", Profile{Username: "u", KeyFile: "k", LocalBase: "/l", RemoteBase: "/r"}},
		{"missing username", Profile{Host: "h", KeyFile: "k", LocalBase: "/l", RemoteBase: "/r"}},
		{"missing key", Profile{Host: "h", Username: "u", LocalBase: "/l", RemoteBase: "/r"}},
		{"missing local", Profile{Host: "h", Username: "u", KeyFile: "k", RemoteBase: "/r"}},
		{"missing remote", Profile{Host: "h", Username: "u", KeyFile: "k", LocalBase: "/l"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateProfile(tc.profile); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateProfileInvalidPort(t *testing.T) {
	p := Profile{Host: "h", Port: 0, Username: "u", KeyFile: "k", LocalBase: "/l", RemoteBase: "/r"}
	if err := ValidateProfile(p); err != nil {
		t.Fatal("port 0 should be allowed (defaults to 22)")
	}
	p.Port = 70000
	if err := ValidateProfile(p); err == nil {
		t.Fatal("expected error for port 70000")
	}
	p.Port = -1
	if err := ValidateProfile(p); err == nil {
		t.Fatal("expected error for port -1")
	}
}

func TestProfileNameValidation(t *testing.T) {
	valid := []string{"production", "staging", "customer-a", "backup_server", "v2.1"}
	for _, name := range valid {
		if err := ValidateProfileName(name); err != nil {
			t.Errorf("expected %q to be valid: %v", name, err)
		}
	}
	invalid := []string{"", "../secret", "a/b", "a\\b", "-bad", ".hidden", "has space"}
	for _, name := range invalid {
		if err := ValidateProfileName(name); err == nil {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestAddDuplicate(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{}}
	p := Profile{Host: "h", Username: "u", KeyFile: "k", LocalBase: "/l", RemoteBase: "/r"}
	if err := cfg.Add("prod", p); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Add("prod", p); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestUpdateAndRemove(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{}}
	p := Profile{Host: "h", Username: "u", KeyFile: "k", LocalBase: "/l", RemoteBase: "/r"}
	if err := cfg.Add("prod", p); err != nil {
		t.Fatal(err)
	}

	updated := p
	updated.Host = "new.example.com"
	if err := cfg.Update("prod", updated); err != nil {
		t.Fatal(err)
	}
	if got, _ := cfg.Get("prod"); got.Host != "new.example.com" {
		t.Fatalf("expected updated host, got %q", got.Host)
	}

	if err := cfg.Update("missing", updated); err == nil {
		t.Fatal("expected error updating missing profile")
	}

	if err := cfg.Remove("prod"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Get("prod"); ok {
		t.Fatal("profile should have been removed")
	}
	if err := cfg.Remove("prod"); err == nil {
		t.Fatal("expected error removing missing profile")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	got := expandHome("~/foo")
	want := filepath.Join(home, "foo")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if expandHome("/absolute/path") != "/absolute/path" {
		t.Fatal("absolute path should be unchanged")
	}
}

func TestCleanRemotePath(t *testing.T) {
	cases := map[string]string{
		"/var/www/app":  "/var/www/app",
		"var/www/app":   "/var/www/app",
		"/var/www/app/": "/var/www/app",
		"//var//www":    "/var/www",
		`\var\www`:      "/var/www",
	}
	for in, want := range cases {
		if got := cleanRemotePath(in); got != want {
			t.Errorf("cleanRemotePath(%q) = %q, want %q", in, got, want)
		}
	}
}
