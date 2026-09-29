// Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause-Clear

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ActiveContext string `yaml:"active_context"`
	Contexts      map[string]Context
}

type Context struct {
	URL   string
	Token string
}

// LoadConfig loads the CLI configuration from the path. If the path is empty,
// it uses the default config path.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = getConfigPath()
		if err != nil {
			return nil, fmt.Errorf("failed to get config path: %w", err)
		}
	}

	data, err := os.ReadFile(path) //gosec:disable G304 // We support file path inclusion for the config.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// GetContext retrieves the context by name. If name is empty, it returns the
// configured active context.
func (c *Config) GetContext(name string) (*Context, error) {
	if c.ActiveContext == "" && name == "" {
		return nil, fmt.Errorf("no default context set")
	} else if name == "" {
		name = c.ActiveContext
	}

	ctx, ok := c.Contexts[name]
	if !ok {
		return nil, fmt.Errorf("context '%s' not found", name)
	}

	if ctx.URL == "" {
		return nil, fmt.Errorf("context '%s' has no URL configured", name)
	}

	if ctx.Token == "" {
		return nil, fmt.Errorf("context '%s' has no token configured", name)
	}

	return &ctx, nil
}

func SaveConfig(configPath string, cfg *Config) error {
	if configPath == "" {
		var err error
		configPath, err = getConfigPath()
		if err != nil {
			return fmt.Errorf("failed to get config path: %w", err)
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func getConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "fiocli.yaml"), nil
}
