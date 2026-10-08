// Package config отвечает за загрузку и разбор конфигурации
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config — корневая структура конфигурации.
// Секции добавляются по мере реализации модулей
type Config struct {
}

// Load читает yaml файл по указанному пути и возвращает Config
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
