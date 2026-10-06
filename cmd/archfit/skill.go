// Package main — `archfit skill install`.
//
// The binary embeds the archfit agent skill (skills/archfit). install writes
// that copy, so the skill an agent reads always matches the binary it runs.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/skills"
)

// skillName is the embedded skill's directory and the installed one's.
const skillName = "archfit"

// SkillCmd groups the embedded-skill commands.
type SkillCmd struct {
	Install SkillInstallCmd `cmd:"" help:"Install the archfit agent skill that matches this binary."`
}

// SkillInstallCmd writes the embedded skill into a skills directory.
type SkillInstallCmd struct {
	Dir   string `help:"Skills directory; the skill is written to <dir>/archfit." default:".claude/skills" type:"path"`
	Force bool   `help:"Overwrite installed files that differ from this binary's copy."`
}

func (*SkillInstallCmd) Help() string {
	return `Install the archfit agent skill (SKILL.md and references) that ships in
this binary, so the skill matches the commands the binary has.

It writes <dir>/archfit. A file that exists with other content is left alone
and the command exits 3, unless --force overwrites it. Files already equal to
this binary's copy are not rewritten.

Examples:
  archfit skill install                       # .claude/skills/archfit
  archfit skill install --dir ~/.claude/skills
  archfit skill install --force`
}

func (c *SkillInstallCmd) Run(deps *appDeps) error {
	dest := filepath.Join(c.Dir, skillName)
	files, err := embeddedSkillFiles()
	if err != nil {
		return &exitError{code: 3, msg: fmt.Sprintf("error: read the embedded skill: %v", err)}
	}
	var conflicts []string
	for rel, content := range files {
		current, err := os.ReadFile(filepath.Join(dest, rel)) //nolint:gosec // a file under the skills dir the user named
		if err == nil && !bytes.Equal(current, content) {
			conflicts = append(conflicts, rel)
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return &exitError{code: 3, msg: fmt.Sprintf("error: read %s: %v", filepath.Join(dest, rel), err)}
		}
	}
	if len(conflicts) > 0 && !c.Force {
		slices.Sort(conflicts)
		return &exitError{code: 3, msg: fmt.Sprintf("error: %s: %s differ from this binary's copy (a local edit, or a copy from another archfit version); rerun with --force to overwrite them",
			dest, strings.Join(conflicts, ", "))}
	}
	written := 0
	for rel, content := range files {
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if current, err := os.ReadFile(target); err == nil && bytes.Equal(current, content) { //nolint:gosec // see above
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
		}
		if err := os.WriteFile(target, content, 0o644); err != nil { //nolint:gosec // skill files are world-readable by design
			return &exitError{code: 3, msg: fmt.Sprintf("error: %v", err)}
		}
		written++
	}
	_, err = fmt.Fprintf(deps.Stdout, "archfit skill %s: %d of %d files written to %s\n", version, written, len(files), dest)
	return err
}

// embeddedSkillFiles maps each embedded skill file, relative to the skill
// root, to its content.
func embeddedSkillFiles() (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(skills.Archfit, skillName, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := fs.ReadFile(skills.Archfit, p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(skillName, filepath.FromSlash(p))
		files[path.Clean(filepath.ToSlash(rel))] = content
		return nil
	})
	return files, err
}
