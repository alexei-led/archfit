package dotnet

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

const (
	csprojExt          = ".csproj"
	slnxExt            = ".slnx"
	slnExt             = ".sln"
	solutionFolderGUID = "2150E333-8FDC-42A3-9474-1A3956D46DE8"
)

// slnProjectLine matches `Project("{type-guid}") = "Name", "path", "{guid}"`.
var slnProjectLine = regexp.MustCompile(`^\s*Project\("\{([0-9A-Fa-f-]+)\}"\)\s*=\s*"[^"]*"\s*,\s*"([^"]*)"\s*,`)

// solutionProjects is the project membership of one solution file. Paths are
// relative to the solution file's directory, slash-separated and cleaned.
type solutionProjects struct {
	// Projects are the .csproj paths, in file order, without duplicates.
	Projects []string
	// Skipped counts project entries that are not .csproj (.sqlproj, .fsproj, ...).
	Skipped int
}

// parseSolution parses a .slnx or .sln by file name.
func parseSolution(name string, data []byte) (solutionProjects, error) {
	switch strings.ToLower(path.Ext(name)) {
	case slnxExt:
		return parseSlnx(data)
	case slnExt:
		return parseSln(data), nil
	default:
		return solutionProjects{}, fmt.Errorf("%s is not a .slnx or .sln file", name)
	}
}

// parseSlnx reads every <Project Path="..."> element at any depth, so projects
// inside <Folder> are found.
func parseSlnx(data []byte) (solutionProjects, error) {
	var out solutionProjects
	seen := map[string]struct{}{}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return solutionProjects{}, fmt.Errorf("parse slnx: %w", err)
		}
		el, ok := tok.(xml.StartElement)
		if !ok || el.Name.Local != "Project" {
			continue
		}
		for _, a := range el.Attr {
			if a.Name.Local == "Path" {
				out.add(a.Value, seen)
			}
		}
	}
}

// parseSln reads `Project(...)` lines. Solution folders are not projects and
// are not counted as skipped.
func parseSln(data []byte) solutionProjects {
	var out solutionProjects
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		m := slnProjectLine.FindStringSubmatch(scanner.Text())
		if m == nil || strings.EqualFold(m[1], solutionFolderGUID) {
			continue
		}
		out.add(m[2], seen)
	}
	return out
}

func (s *solutionProjects) add(raw string, seen map[string]struct{}) {
	p := path.Clean(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"))
	if !strings.EqualFold(path.Ext(p), csprojExt) {
		s.Skipped++
		return
	}
	if _, dup := seen[p]; dup {
		return
	}
	seen[p] = struct{}{}
	s.Projects = append(s.Projects, p)
}
