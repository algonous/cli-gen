package codegen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/autonous/cli-gen/internal/schema"
)

func TestRenderMain(t *testing.T) {
	set := &schema.SchemaSet{
		CLI: &schema.CliFile{Cli: schema.CliDef{Name: "github", Description: "GitHub REST API CLI"}},
		Actions: []*schema.ActionFile{
			{Name: "list-repo-issues"},
			{Name: "create-issue"},
			{Name: "create-or-update-file"},
		},
	}

	out, err := RenderMain(set)
	if err != nil {
		t.Fatalf("RenderMain error: %v", err)
	}

	for _, part := range []string{"list-repo-issues", "create-issue", "create-or-update-file", "skill"} {
		if !strings.Contains(out, part) {
			t.Fatalf("rendered output missing %q", part)
		}
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "generated_main.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}
