package main

import (
	"os"
	"testing"
)

// isolateConfigDir перенаправляє os.UserConfigDir у тимчасову теку на всіх ОС.
func isolateConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_MODEL", "")
}

func TestConfigRoundTrip(t *testing.T) {
	isolateConfigDir(t)
	if c := loadConfigFile(); c != (Config{}) {
		t.Fatalf("expected empty config, got %+v", c)
	}
	want := Config{APIKey: "sk-or-test", Models: "a:free, b:free"}
	path, err := saveConfigFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := loadConfigFile(); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("config must be 0600, got %v (%v)", info.Mode().Perm(), err)
	}
}

func TestEffectiveConfigPriority(t *testing.T) {
	isolateConfigDir(t)
	t.Setenv("OPENROUTER_API_KEY", "env-key")
	t.Setenv("OPENROUTER_MODEL", "env-model")

	if c := effectiveConfig(""); c.APIKey != "env-key" || c.Models != "env-model" {
		t.Errorf("env fallback: %+v", c)
	}
	saveConfigFile(Config{APIKey: "file-key", Models: "file-model"})
	if c := effectiveConfig(""); c.APIKey != "file-key" || c.Models != "file-model" {
		t.Errorf("file must override env: %+v", c)
	}
	if c := effectiveConfig("flag-model"); c.Models != "flag-model" {
		t.Errorf("flag must override file: %+v", c)
	}
}
