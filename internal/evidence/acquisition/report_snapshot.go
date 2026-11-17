package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alexei-led/archfit/v4/internal/factcache"
	historygit "github.com/alexei-led/archfit/v4/internal/history/git"
	"github.com/alexei-led/archfit/v4/internal/model/evidence"
	"github.com/alexei-led/archfit/v4/internal/toolrun"
)

// ReportSnapshotSource fingerprints a declared live subset, not build closure.
// It writes no cache, index or objects. ListInputs retains its entry/depth caps.
type ReportSnapshotSource struct {
	Root, PolicyPath string
	Exclude, Inputs  []string
	Runner           toolrun.Runner
}

const reportSnapshotBytes = 32 << 20

// Capture re-enumerates and hashes raw bytes, retaining hashes for references.
func (s ReportSnapshotSource) Capture(ctx context.Context) evidence.ReportFileSnapshot {
	out := evidence.ReportFileSnapshot{Files: map[string]string{}, Cleanliness: "unknown", Unknowns: []evidence.ReportUnknownFact{}}
	gap := func(fact, reason string) {
		out.Unknowns = append(out.Unknowns, evidence.ReportUnknownFact{Fact: fact, Reason: reason, Owner: "evidence/acquisition"})
	}
	gap("input_closure", "only named source/module/build inputs are captured; ignored and external build inputs and transient ABA edits are not tracked")
	if len(s.Exclude) > 0 {
		gap("excluded_inputs", "policy and built-in exclusions bound the subset, not the complete architecture")
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		gap("snapshot", "source root cannot be opened")
		return out
	}
	defer func() { _ = root.Close() }()
	physicalRoot, err := filepath.EvalSymlinks(s.Root)
	if err != nil {
		gap("snapshot", "source root cannot be resolved")
		return out
	}
	match := factcache.MatchExts([]string{".go", ".rs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".pyi", ".cs", ".fs", ".vb"},
		[]string{"go.mod", "go.sum", "go.work", "go.work.sum", "Cargo.toml", "Cargo.lock", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "tsconfig.json", "pyproject.toml", "uv.lock", "requirements.txt", "Directory.Build.props", "Directory.Build.targets", "global.json", "NuGet.Config"})
	extra := slices.Clone(s.Inputs)
	isInput := func(rel string) bool {
		ext := filepath.Ext(rel)
		return match(rel) || ext == ".csproj" || ext == ".fsproj" || ext == ".vbproj" || ext == ".sln" || slices.Contains(extra, rel)
	}
	exclusions := append(slices.Clone(s.Exclude), "**/.env*", ".env*", "**/.ssh/**", ".ssh/**", "**/.aws/**", ".aws/**")
	complete := true
	for _, exclusion := range s.Exclude {
		if strings.HasPrefix(exclusion, "!") {
			gap("snapshot", "re-included paths are not fully enumerated by the fingerprint helper")
			complete = false
		}
	}
	paths := []string{}
	note := ""
	if err := reportCheckSymlinks(s.Root); err != nil {
		gap("snapshot", err.Error())
		complete = false
	} else {
		paths, note = factcache.ListInputs(s.Root, isInput, exclusions)
	}
	if note != "" {
		gap("snapshot", note)
		complete = false
	}
	policyRel, err := filepath.Rel(s.Root, s.PolicyPath)
	if err != nil || !reportRelative(policyRel) {
		gap("policy_bytes", "policy is outside the source root")
		complete = false
	} else {
		paths = append(paths, filepath.ToSlash(policyRel))
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	for _, input := range extra {
		if !reportRelative(input) || reportSecretPath(input) || !slices.Contains(paths, filepath.ToSlash(input)) {
			gap("external_input", "declared input is excluded, missing, external or sensitive: "+input)
			complete = false
		}
	}
	s.captureFiles(ctx, root, physicalRoot, policyRel, paths, complete, &out, gap)
	if s.Runner != nil {
		if head, e := historygit.HeadRef(ctx, s.Root, s.Runner); e == nil {
			out.HeadCommit = &head
		} else {
			gap("head_commit", "Git HEAD unavailable")
		}
		if tree, e := historygit.TreeID(ctx, s.Root, s.Runner); e == nil {
			out.HeadTree = &tree
		} else {
			gap("head_tree", "Git HEAD tree unavailable")
		}
		result, e := s.Runner.Run(ctx, toolrun.ToolCmd{Name: "git", Args: []string{"--no-optional-locks", "status", "--porcelain", "--untracked-files=all"}, WorkDir: s.Root})
		if e == nil && result.ExitCode == 0 {
			out.Cleanliness = "clean"
			for _, line := range strings.Split(string(result.Stdout), "\n") {
				if line != "" && !strings.HasPrefix(strings.Trim(line[min(3, len(line)):], "\""), ".archfit-cache/") {
					out.Cleanliness = "dirty"
					break
				}
			}
		} else {
			gap("cleanliness", "Git status unavailable")
		}
	} else {
		gap("git_identity", "Git runner unavailable")
	}
	return out
}

func reportRelative(rel string) bool {
	return rel != "" && rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func reportSecretPath(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(part, ".env") || part == ".ssh" || part == ".aws" || part == ".netrc" || part == ".npmrc" || part == "credentials.json" {
			return true
		}
	}
	return false
}

func (s ReportSnapshotSource) captureFiles(ctx context.Context, root *os.Root, physicalRoot, policyRel string, paths []string, complete bool, out *evidence.ReportFileSnapshot, gap func(string, string)) {
	remaining := int64(reportSnapshotBytes)
	ordered := make([]string, 0, len(paths))
	policyName := filepath.ToSlash(policyRel)
	if slices.Contains(paths, policyName) {
		ordered = append(ordered, policyName)
	}
	for _, path := range paths {
		if path != policyName {
			ordered = append(ordered, path)
		}
	}
	for _, rel := range ordered {
		if ctx.Err() != nil {
			gap("snapshot", "capture cancelled")
			complete = false
			break
		}
		if !reportRelative(rel) {
			gap("snapshot", "enumeration incomplete or invalid path")
			complete = false
			continue
		}
		target, e := filepath.EvalSymlinks(filepath.Join(s.Root, filepath.FromSlash(rel)))
		targetRel := ""
		if e == nil {
			targetRel, e = filepath.Rel(physicalRoot, target)
		}
		if e != nil || !reportRelative(targetRel) || reportSecretPath(targetRel) {
			gap("file_bytes", "unresolved, escaping or secret-path input: "+rel)
			complete = false
			continue
		}
		f, e := root.Open(filepath.FromSlash(rel))
		if e != nil {
			gap("file_bytes", "cannot open input: "+rel)
			complete = false
			continue
		}
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() {
			_ = f.Close()
			gap("snapshot", "enumeration incomplete or nonregular input: "+rel)
			complete = false
			continue
		}
		content := sha256.New()
		n, e := io.Copy(content, io.LimitReader(f, remaining+1))
		_ = f.Close()
		remaining -= n
		if e != nil || remaining < 0 {
			gap("file_bytes", "read failed or raw-byte budget exceeded: "+rel)
			complete = false
			break
		}
		sum := content.Sum(nil)
		digest := "sha256:" + hex.EncodeToString(sum)
		out.Files[rel] = digest
		if rel == filepath.ToSlash(policyRel) {
			out.PolicyDigest = new(digest)
		}
	}
	if complete {
		h := sha256.New()
		for _, rel := range paths {
			digest, err := hex.DecodeString(strings.TrimPrefix(out.Files[rel], "sha256:"))
			if err != nil || len(digest) != sha256.Size {
				complete = false
				gap("snapshot", "a captured file digest is unavailable")
				break
			}
			_, _ = h.Write([]byte(rel))
			_, _ = h.Write([]byte{0})
			_, _ = h.Write(digest)
			_, _ = h.Write([]byte{0})
		}
		if complete {
			out.Digest = new("sha256:" + hex.EncodeToString(h.Sum(nil)))
		}
	}
}
