package main

func reportLanguageMode(mode string) string {
	switch mode {
	case "on":
		return "true"
	case "off":
		return "false"
	default:
		return "auto"
	}
}
