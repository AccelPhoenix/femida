// Package config отвечает за загрузку и разбор конфигурации
package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/accelphoenix/femida/internal/tftp"
)

// Config — корневая структура конфигурации.
// Агрегирует секции модулей Femida.
type Config struct {
	TFTP tftp.Config `yaml:"tftp"`
}

// Load читает yaml файл по указанному пути и возвращает Config
func Load(path string) (*Config, error) {
	cfg := Config{
		TFTP: tftp.DefaultConfig(),
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &cfg, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
