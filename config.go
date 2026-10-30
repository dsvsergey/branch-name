package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config — налаштування, збережені у файлі (формат як у .env).
type Config struct {
	APIKey string
	Models string // через кому; порожньо — моделі за замовчуванням
}

// configPath: %AppData%\branch-name\config.env на Windows,
// ~/Library/Application Support/branch-name/config.env на macOS, ~/.config/... на Linux.
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "branch-name", "config.env"), nil
}

func loadConfigFile() Config {
	var c Config
	path, err := configPath()
	if err != nil {
		return c
	}
	f, err := os.Open(path)
	if err != nil {
		return c
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "OPENROUTER_API_KEY":
			c.APIKey = v
		case "OPENROUTER_MODEL":
			c.Models = v
		}
	}
	return c
}

func saveConfigFile(c Config) (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	content := fmt.Sprintf("# branch-name settings\nOPENROUTER_API_KEY=%s\nOPENROUTER_MODEL=%s\n",
		strings.TrimSpace(c.APIKey), strings.TrimSpace(c.Models))
	// 0600: ключ доступний лише власнику файлу
	return path, os.WriteFile(path, []byte(content), 0o600)
}

// effectiveConfig: прапорець -model > файл налаштувань > змінні оточення.
func effectiveConfig(modelFlag string) Config {
	file := loadConfigFile()
	return Config{
		APIKey: firstNonEmpty(file.APIKey, os.Getenv("OPENROUTER_API_KEY")),
		Models: firstNonEmpty(modelFlag, file.Models, os.Getenv("OPENROUTER_MODEL")),
	}
}
