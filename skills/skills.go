// Package skills embeds the agent skills that ship with the archfit binary, so
// `archfit skill install` installs the copy that matches the binary version.
package skills

import "embed"

// Archfit is the archfit agent skill: SKILL.md and its references, rooted at
// "archfit".
//
//go:embed archfit
var Archfit embed.FS
