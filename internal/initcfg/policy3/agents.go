package policy3

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
)

// Markers of the instruction block. They are the markers the Archfit 3
// agents-md command wrote, so Init replaces an old block.
const (
	markerStart = "<!-- archfit:start -->"
	markerEnd   = "<!-- archfit:end -->"
)

// Block is the instruction block for agents: the five lines of design 11.3
// between the markers.
const Block = markerStart + "\n" +
	"Run `archfit rules` before you start and `archfit check` before you finish.\n" +
	"Do what `next:` says. `archfit: pass` means done.\n" +
	"Before a new dependency between modules: `archfit can <from> <to>`.\n" +
	"If a rule is in your way: `archfit request -m \"<why>\"`, then wait or do other work.\n" +
	"Never edit `.archfit.yaml`. Never copy code to get around a rule.\n" +
	markerEnd + "\n"

// spliceFile returns the content of the instruction file at path with Block in
// it. A missing file gives Block alone.
func spliceFile(path string) ([]byte, error) {
	current, err := os.ReadFile(path) //nolint:gosec // the instruction file of the repository init writes into
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return splice(current)
}

// splice replaces the text between the markers, markers included, with Block,
// or appends Block after a blank line when current has no markers. Every byte
// outside the markers stays as it was. Anything but one start marker before
// one end marker, or none of either, is an error.
func splice(current []byte) ([]byte, error) {
	starts, ends := bytes.Count(current, []byte(markerStart)), bytes.Count(current, []byte(markerEnd))
	start := bytes.Index(current, []byte(markerStart))
	end := bytes.Index(current, []byte(markerEnd))
	switch {
	case starts == 0 && ends == 0:
		out := slices.Clone(current)
		switch {
		case len(out) == 0:
		case bytes.HasSuffix(out, []byte("\n")):
			out = append(out, '\n')
		default:
			out = append(out, "\n\n"...)
		}
		return append(out, Block...), nil
	case starts != 1 || ends != 1 || end < start:
		return nil, errors.New("the archfit markers are unbalanced: keep one " + markerStart + " before one " + markerEnd)
	}
	end += len(markerEnd)
	// The newline that ends Block replaces the one after the old end marker;
	// without one there (end of file, CRLF), Block ends at its marker too.
	block := Block
	if end < len(current) && current[end] == '\n' {
		end++
	} else {
		block = strings.TrimSuffix(Block, "\n")
	}
	out := append(slices.Clone(current[:start]), block...)
	return append(out, current[end:]...), nil
}
