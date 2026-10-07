package evaluation

import "strings"

// validationCommand returns the shell command a repair task should run to
// verify its fix. It is part of the repair contract: an agent that applies an
// agent_task re-runs exactly this command to confirm the finding is gone.
func validationCommand(configPath, root string, extraArgs ...string) string {
	args := []string{"archfit", "check", "-c", configPath}
	if root != "" {
		args = append(args, "--root", root)
	}
	args = append(args, extraArgs...)
	for i := range args {
		args[i] = ShellQuoteArg(args[i])
	}
	return strings.Join(args, " ")
}

// ShellQuoteArg quotes one argument of a validation command for a POSIX
// shell. A caller that rewrites a path inside the command matches and writes
// the argument in this form, so the command stays one argument per path.
func ShellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$`!#&;()*<>?[\\]^{|}~") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}
