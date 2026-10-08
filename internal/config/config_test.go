package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig — helper: создаёт файл с заданным содержимым и возвращает путь.
func writeConfig(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// TestLoadMissingFileReturnDefaults проверяет, что при отсутствии файла
// возвращаются дефолты без ошибки.
func TestLoadMissingFileReturnDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if cfg == nil {
		t.Fatal("want non-nil config, got nil")
	}
	if cfg.TFTP.Port != 69 {
		t.Errorf("want default port 69, got %d", cfg.TFTP.Port)
	}
	if cfg.TFTP.AssetsPath == "" {
		t.Error("want non-empty default AssetsPath")
	}
}

// TestLoadInvalidYAML проверяет, что невалидный YAML возвращает ошибку.
func TestLoadInvalidYAML(t *testing.T) {
	path := writeConfig(t, "invalid.yaml", "example: [this is not valid")

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// TestLoadEmptyFile проверяет, что пустой файл даёт дефолты без ошибки.
func TestLoadEmptyFile(t *testing.T) {
	path := writeConfig(t, "empty.yaml", "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if cfg.TFTP.Port != 69 {
		t.Errorf("want default port 69, got %d", cfg.TFTP.Port)
	}
	if cfg.TFTP.AssetsPath == "" {
		t.Error("want non-empty default AssetsPath")
	}
}

// TestLoadCommentsOnly проверяет, что YAML только с комментариями
// даёт дефолты без ошибки.
func TestLoadCommentsOnly(t *testing.T) {
	content := "# только комментарий\n# и ещё один\n"
	path := writeConfig(t, "comments.yaml", content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TFTP.Port != 69 {
		t.Errorf("want default port 69, got %d", cfg.TFTP.Port)
	}
}

// TestLoadFullYAML проверяет, что полная секция TFTP парсится корректно.
func TestLoadFullYAML(t *testing.T) {
	content := `tftp:
  port: 6969
  assets_path: /srv/assets
`
	path := writeConfig(t, "full.yaml", content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TFTP.Port != 6969 {
		t.Errorf("port: want 6969, got %d", cfg.TFTP.Port)
	}
	if cfg.TFTP.AssetsPath != "/srv/assets" {
		t.Errorf("assets_path: want /srv/assets, got %q", cfg.TFTP.AssetsPath)
	}
}

// TestLoadPartialYAML проверяет, что при указании только port
// поле assets_path остаётся дефолтным.
func TestLoadPartialYAML(t *testing.T) {
	content := "tftp:\n  port: 6969\n"
	path := writeConfig(t, "partial.yaml", content)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TFTP.Port != 6969 {
		t.Errorf("port: want 6969, got %d", cfg.TFTP.Port)
	}
	if cfg.TFTP.AssetsPath == "" {
		t.Error("assets_path: want default (non-empty), got empty")
	}
}
