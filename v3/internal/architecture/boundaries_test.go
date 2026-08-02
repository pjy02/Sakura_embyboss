package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProcessDependencyBoundaries(t *testing.T) {
	root := repositoryRoot(t)
	tests := []struct {
		directory string
		forbidden []string
	}{
		{
			directory: "cmd/bot",
			forbidden: []string{"internal/postgres", "internal/redisstore", "internal/migrate"},
		},
		{
			directory: "cmd/api",
			forbidden: []string{"cmd/bot", "internal/migrate"},
		},
		{
			directory: "cmd/worker",
			forbidden: []string{"cmd/bot", "internal/migrate"},
		},
	}
	for _, test := range tests {
		t.Run(test.directory, func(t *testing.T) {
			imports := importsInDirectory(t, filepath.Join(root, test.directory))
			for _, imported := range imports {
				for _, forbidden := range test.forbidden {
					if strings.Contains(imported, forbidden) {
						t.Fatalf("%s must not import %s", test.directory, imported)
					}
				}
			}
		})
	}
}

func TestWebUsesGeneratedAPIClientWithoutServerBusinessImports(t *testing.T) {
	root := repositoryRoot(t)
	sourceRoot := filepath.Join(root, "web", "src")
	generated, err := os.ReadFile(filepath.Join(sourceRoot, "generated", "client.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "Code generated from api/openapi.yaml") {
		t.Fatal("Web API client must be generated from the OpenAPI contract")
	}
	err = filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".vue")) || strings.Contains(filepath.ToSlash(path), "/generated/") {
			return walkErr
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, forbidden := range []string{"fetch(", "axios.", "internal/platform", "internal/identity", "SELECT ", "INSERT INTO ", "UPDATE accounts"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s bypasses the generated API/business boundary with %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOnlyMigrateEntrypointOwnsMigrationRunner(t *testing.T) {
	root := repositoryRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "cmd", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range matches {
		imports := importsInDirectory(t, directory)
		usesMigrate := false
		for _, imported := range imports {
			usesMigrate = usesMigrate || strings.Contains(imported, "internal/migrate")
		}
		name := filepath.Base(directory)
		if name == "migrate" && !usesMigrate {
			t.Fatal("migrate entrypoint must own the migration runner")
		}
		if name != "migrate" && usesMigrate {
			t.Fatalf("%s entrypoint must not run migrations", name)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func importsInDirectory(t *testing.T, directory string) []string {
	t.Helper()
	packages, err := parser.ParseDir(token.NewFileSet(), directory, func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", directory, err)
	}
	var imports []string
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				item, ok := node.(*ast.ImportSpec)
				if !ok {
					return true
				}
				value, err := strconv.Unquote(item.Path.Value)
				if err == nil {
					imports = append(imports, value)
				}
				return false
			})
		}
	}
	return imports
}
