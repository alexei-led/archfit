package dotnet

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	slnfExt            = ".slnf"
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
	// Base is the solution a solution filter (.slnf) narrows, relative to the
	// filter's directory; empty for a .slnx or .sln.
	Base string
}

// parseSolution parses a .slnx, .sln, or .slnf by file name.
func parseSolution(name string, data []byte) (solutionProjects, error) {
	switch strings.ToLower(path.Ext(name)) {
	case slnxExt:
		return parseSlnx(data)
	case slnExt:
		return parseSln(data), nil
	case slnfExt:
		return parseSlnf(data)
	default:
		return solutionProjects{}, fmt.Errorf("%s is not a .slnx, .sln, or .slnf file", name)
	}
}

// parseSlnf reads a solution filter: the base solution's path, relative to the
// filter, and the projects it keeps, relative to the base solution. The
// projects are returned relative to the filter's directory, as for the other
// formats.
func parseSlnf(data []byte) (solutionProjects, error) {
	var doc struct {
		Solution struct {
			Path     string   `json:"path"`
			Projects []string `json:"projects"`
		} `json:"solution"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &doc); err != nil {
		return solutionProjects{}, fmt.Errorf("parse slnf: %w", err)
	}
	if doc.Solution.Path == "" {
		return solutionProjects{}, errors.New("parse slnf: solution.path is empty")
	}
	out := solutionProjects{Base: slashClean(doc.Solution.Path)}
	dir := path.Dir(out.Base)
	seen := map[string]struct{}{}
	for _, p := range doc.Solution.Projects {
		out.add(path.Join(dir, slashClean(p)), seen)
	}
	return out, nil
}

func slashClean(raw string) string {
	return path.Clean(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"))
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
	p := slashClean(raw)
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
