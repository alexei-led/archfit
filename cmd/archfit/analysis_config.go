package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/alexei-led/archfit/internal/config"
)

// loadConfig loads command configuration and validates rule definitions. The
// default config is optional for commands that historically use config.Default.
func loadConfig(ctx context.Context, path string) (config.Config, error) {
	cfg, err := config.Load(ctx, path)
	if err != nil {
		if path == defaultConfigPath && errors.Is(err, os.ErrNotExist) {
			return config.Default(), nil
		}
		return config.Config{}, err
	}
	if err := config.ValidateRules(cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// loadAnalysisConfig loads a required config for analysis commands and maps
// missing files to the command's input-error contract.
func loadAnalysisConfig(ctx context.Context, path string) (config.Config, error) {
	cfg, err := config.Load(ctx, path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Config{}, &exitError{code: 3, msg: fmt.Sprintf("error: config not found: %s\n→ run: archfit config init --root .", path)}
		}
		return config.Config{}, err
	}
	if err := config.ValidateRules(cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// configLoadError normalises a config-load failure into an exit-3 error.
func configLoadError(err error) error {
	var already *exitError
	if errors.As(err, &already) {
		return already
	}
	return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
}
