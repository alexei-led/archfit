package main

import (
	"github.com/alexei-led/archfit/internal/config"
	"github.com/alexei-led/archfit/internal/extract/acquire"
	"github.com/alexei-led/archfit/internal/extract/registry"
)

func extractConfigs(cfg config.Config) registry.Configs {
	return registry.Configs{
		config.LangGo:         cfg.ForExtract(config.LangGo),
		config.LangTypeScript: cfg.ForExtract(config.LangTypeScript),
		config.LangPython:     cfg.ForExtract(config.LangPython),
		config.LangRust:       cfg.ForExtract(config.LangRust),
	}
}

func acquisitionOptions(cfg config.Config) acquire.Options {
	return acquire.Options{
		Exclusions: cfg.Exclude, FileClass: cfg.ForFileClass(), ModuleMap: cfg.ModuleMapView(),
		ClonesEnabled: cfg.ClonesEnabled(), CloneTimeout: cfg.ToolTimeout(config.ToolClones),
		SCIPEnabled: cfg.ScipEnabled(), SCIPTimeout: cfg.ToolTimeout(config.ToolScip),
		Syntax: cfg.ForSyntax(), GoExtract: cfg.ForExtract(config.LangGo),
	}
}
