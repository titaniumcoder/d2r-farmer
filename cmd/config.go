package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider string       `yaml:"provider"`
	OpenAI   OpenAIConfig `yaml:"openai"`
}

type OpenAIConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

func configPath() string {
	return filepath.Join("data", "config.yaml")
}

func writeConfig(cfg Config) error {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	content, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath(), content, 0o600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

func readConfig() (Config, error) {
	content, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("missing config at %s (run `d2r-farmer init` first)", configPath())
		}
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.OpenAI.APIKey = strings.TrimSpace(cfg.OpenAI.APIKey)
	cfg.OpenAI.Model = strings.TrimSpace(cfg.OpenAI.Model)

	if cfg.Provider == "" {
		return Config{}, fmt.Errorf("config provider is empty")
	}

	if cfg.Provider != "openai" {
		return Config{}, fmt.Errorf("unsupported provider %q (only openai is supported for now)", cfg.Provider)
	}

	if cfg.OpenAI.APIKey == "" {
		return Config{}, fmt.Errorf("openai.api_key is empty in %s", configPath())
	}

	if cfg.OpenAI.Model == "" {
		cfg.OpenAI.Model = "gpt-4.1-mini"
	}

	return cfg, nil
}
