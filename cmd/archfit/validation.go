package main

import "slices"

const requireToolsFlag = "--require-tools"

func scanValidationArgs(req scanRequest) []string {
	var args []string
	if req.baseRef != "" {
		args = append(args, "--base", req.baseRef)
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
