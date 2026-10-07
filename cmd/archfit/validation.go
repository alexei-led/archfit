package main

import (
	"cmp"
	"slices"
)

const requireToolsFlag = "--require-tools"

func scanValidationArgs(req scanRequest) []string {
	var args []string
	if base := cmp.Or(req.validationBase, req.baseRef); base != "" {
		args = append(args, "--base", base)
	}
	languages := slices.Clone(req.lang)
	slices.Sort(languages)
	for _, language := range slices.Compact(languages) {
		args = append(args, "--lang", language)
	}
	if req.requireTools {
		args = append(args, requireToolsFlag)
	}
	return args
}
