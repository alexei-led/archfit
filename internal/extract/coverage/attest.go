package coverage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexei-led/archfit/internal/model/evidence"
)

const (
	attestationSchemaVersion  = 1
	reasonFreshnessUnverified = "freshness_unverified"
	reasonWorktreeDiffers     = "worktree_differs_from_ref"
)

type attestationSidecar struct {
	SchemaVersion int               `json:"schema_version"`
	SourceRef     string            `json:"source_ref"`
	Modules       []string          `json:"modules"`
	Sources       map[string]string `json:"sources"`
}

type attestationResult struct {
	freshness evidence.CoverageFreshness
	reason    string
	sourceRef string
}

// attest compares only the producer-enumerated covered source universe against
// the bytes under the scan root. It never asks git, walks for extra source files,
// or treats source_ref as proof about a worktree.
func attest(normalizer *Normalizer, sidecarPath string, maxBytes int64) attestationResult {
	unverified := attestationResult{freshness: evidence.FreshnessUnverified, reason: reasonFreshnessUnverified}
	path, err := resolveContainedFile(normalizer.root, sidecarPath)
	if err != nil {
		return unverified
	}
	data, err := readBoundedFile(path, maxBytes)
	if err != nil {
		if errors.Is(err, errCoverageInputTooLarge) {
			unverified.reason = joinReasons(unverified.reason, reasonCoverageSidecarTooLarge)
		}
		return unverified
	}
	var sidecar attestationSidecar
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&sidecar); err != nil {
		return unverified
	}
	if err := ensureJSONEOF(dec); err != nil || sidecar.SchemaVersion != attestationSchemaVersion {
		return unverified
	}
	for _, module := range sidecar.Modules {
		if strings.TrimSpace(module) == "" {
			return unverified
		}
	}
	sourcePaths := make([]string, 0, len(sidecar.Sources))
	for rawPath := range sidecar.Sources {
		sourcePaths = append(sourcePaths, rawPath)
	}
	sort.Strings(sourcePaths)
	for _, rawPath := range sourcePaths {
		wantHash := sidecar.Sources[rawPath]
		if !validSHA256(wantHash) {
			return unverified
		}
		rel, err := normalizer.Normalize(rawPath)
		if err != nil {
			return attestationResult{freshness: evidence.FreshnessStale, reason: reasonWorktreeDiffers, sourceRef: sidecar.SourceRef}
		}
		contents, err := readBoundedFile(filepath.Join(normalizer.root, filepath.FromSlash(rel)), maxBytes)
		if err != nil {
			reason := reasonWorktreeDiffers
			if errors.Is(err, errCoverageInputTooLarge) {
				reason = joinReasons(reason, reasonCoverageAttestedSourceTooLarge)
			}
			return attestationResult{freshness: evidence.FreshnessStale, reason: reason, sourceRef: sidecar.SourceRef}
		}
		sum := sha256.Sum256(contents)
		if hex.EncodeToString(sum[:]) != wantHash {
			return attestationResult{freshness: evidence.FreshnessStale, reason: reasonWorktreeDiffers, sourceRef: sidecar.SourceRef}
		}
	}
	return attestationResult{freshness: evidence.FreshnessMatched, sourceRef: sidecar.SourceRef}
}

func ensureJSONEOF(dec *json.Decoder) error {
	var trailing any
	err := dec.Decode(&trailing)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return errors.New("trailing JSON value")
	}
	return err
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
