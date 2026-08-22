package notes

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionFilesDoNotImportPresentationConfigOrCharm(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", file, err)
			}
			if strings.HasPrefix(path, "charm.land/") ||
				path == "github.com/gonfff/jotmd/internal/config" || strings.HasPrefix(path, "github.com/gonfff/jotmd/internal/config/") ||
				path == "github.com/gonfff/jotmd/internal/presentation" || strings.HasPrefix(path, "github.com/gonfff/jotmd/internal/presentation/") ||
				path == "github.com/gonfff/jotmd/internal/ui" || strings.HasPrefix(path, "github.com/gonfff/jotmd/internal/ui/") {
				t.Errorf("%s imports forbidden %q; internal/notes must not depend on presentation, config, or charm.land packages", file, path)
			}
		}
	}
}
