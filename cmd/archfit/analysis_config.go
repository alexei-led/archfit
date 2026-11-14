package main

import (
	"errors"
	"os"

	"github.com/alexei-led/archfit/v3/internal/config"
)

// loadConfig reads doctor configuration. An absent default policy is optional.
func loadConfig(path string) (config.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the policy
	if err != nil {
		if path == defaultConfigPath && errors.Is(err, os.ErrNotExist) {
			return config.Default(), nil
		}
		return config.Config{}, err
	}
	return config.ReportConfig(data)
}
