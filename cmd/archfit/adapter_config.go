package main

import (
	apppipeline "github.com/alexei-led/archfit/internal/analysispipeline"
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/registry"
)

func extractConfigs(cfg config.Config) registry.Configs { return apppipeline.ExtractConfigs(cfg) }
