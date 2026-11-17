// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Metadata codecs and journal audits must remain independent of SVG rendering.
// Actual source authentication belongs to the concrete gymstable selection.
func TestEntryMetadataHasNoPortraitRendererDependency(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "github.com/taytr-llc/slug/fighterportrait" {
				t.Fatalf("%s imports the SVG renderer into entry metadata validation", name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no runtime sources inspected")
	}
}
