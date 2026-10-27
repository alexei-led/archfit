package dotnet

import (
	"slices"
	"testing"
)

const slnxName = "A.slnx"

func TestParseSolution(t *testing.T) {
	const slnxFolders = `<Solution>
  <Folder Name="/src/">
    <Project Path="src/Core/Core.csproj" />
    <Project Path="src/Data/Data.sqlproj" />
  </Folder>
  <Project Path="tests\Core.Tests\Core.Tests.csproj" />
  <Project Path="src/Core/Core.csproj" />
</Solution>`
	const sln = "\xef\xbb\xbf\r\nMicrosoft Visual Studio Solution File, Format Version 12.00\r\n" +
		`Project("{2150E333-8FDC-42A3-9474-1A3956D46DE8}") = "src", "src", "{11111111-1111-1111-1111-111111111111}"` + "\r\n" +
		"EndProject\r\n" +
		`Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "Core", "src\Core\Core.csproj", "{22222222-2222-2222-2222-222222222222}"` + "\r\n" +
		"EndProject\r\n" +
		`Project("{00D1A9C2-B5F0-4AF3-8072-F6C62B6B5C4C}") = "Data", "src\Data\Data.sqlproj", "{33333333-3333-3333-3333-333333333333}"` + "\r\n" +
		"EndProject\r\n" +
		`Project("{2150e333-8fdc-42a3-9474-1a3956d46de8}") = "lower", "lower", "{44444444-4444-4444-4444-444444444444}"` + "\r\n" +
		"EndProject\r\n"

	tests := []struct {
		name        string
		file        string
		data        string
		wantErr     bool
		wantPaths   []string
		wantSkipped int
	}{
		{
			name:        "slnx with folder, backslash path, duplicate and sqlproj",
			file:        slnxName,
			data:        slnxFolders,
			wantPaths:   []string{"src/Core/Core.csproj", "tests/Core.Tests/Core.Tests.csproj"},
			wantSkipped: 1,
		},
		{
			name:        "sln skips solution folders and non-csproj, converts backslashes, strips BOM",
			file:        "A.sln",
			data:        sln,
			wantPaths:   []string{"src/Core/Core.csproj"},
			wantSkipped: 1,
		},
		{name: "empty slnx", file: slnxName, data: `<Solution/>`},
		{name: "malformed slnx", file: slnxName, data: `<Solution><Project Path="a.csproj"`, wantErr: true},
		{name: "unknown extension", file: "A.txt", data: ``, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSolution(tt.file, []byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !slices.Equal(got.Projects, tt.wantPaths) {
				t.Errorf("projects = %v, want %v", got.Projects, tt.wantPaths)
			}
			if got.Skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", got.Skipped, tt.wantSkipped)
			}
		})
	}
}
