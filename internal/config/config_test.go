package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadFileNotFound проверяет, что отсутсвующий файл выдаёт ошибку
func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestLoadInvalidYaml проверяет, что невалидный YAML выдаёт ошибку
func TestLoadInvalidYaml(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")

	content := "example: [this is not valid"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// TestLoadEmptyConfig проверяет, что пустой YAML даёт пустую Config без ошибки.
func TestLoadEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")

	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	cfg, err := Load(path)

	if err != nil {
		t.Fatalf("load empty config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expeted non-nil config, got nil")
	}
}

// TestLoadEmptyYAML проверяет, что YAML с комментариями, но без данных,
// даёт пустой Config.
func TestLoadEmptyYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "comments.yaml")

	content := "# только комментарий\n# и ещё один\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}
