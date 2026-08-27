package evaluation

import "strings"

// validationCommand returns the shell command a repair task should run to
// verify its fix. It is part of the repair contract: an agent that applies an
// agent_task re-runs exactly this command to confirm the finding is gone.
func validationCommand(configPath, root string) string {
	args := []string{"archfit", "check", "-c", configPath}
	if root != "" {
		args = append(args, "--root", root)
	}
	for i := range args {
		args[i] = shellQuoteArg(args[i])
	}
	return strings.Join(args, " ")
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$`!#&;()*<>?[\\]^{|}~") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}
