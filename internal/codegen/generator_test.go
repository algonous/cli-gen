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

func TestRenderArgParser(t *testing.T) {
	action := &schema.ActionFile{
		Name: "list-repo-issues",
		Args: []schema.ArgDef{
			{Name: "owner", Type: "string", Required: true, Help: "owner"},
			{Name: "repo", Type: "string", Required: true, Help: "repo"},
			{Name: "state", Type: "string", Required: false, Help: "state"},
			{Name: "labels", Type: "array", Required: false, Help: "labels"},
		},
	}
	out, err := RenderArgParser(action)
	if err != nil {
		t.Fatalf("RenderArgParser error: %v", err)
	}
	for _, part := range []string{
		`fs.String("owner"`,
		`fs.String("repo"`,
		`fs.String("state"`,
		`arrayFlag`,
		`short flags are not supported`,
		`fs.Visit(func(f *flag.Flag)`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("rendered output missing %q", part)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_arg_parser.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}
