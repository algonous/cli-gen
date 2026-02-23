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

func TestRenderRequestBuilder(t *testing.T) {
	cli := &schema.CliFile{
		Cli: schema.CliDef{
			Runtime: schema.RuntimeDef{
				Env: []schema.EnvEntry{
					{Name: "GITHUB_BASE_URL", Required: true},
					{Name: "GITHUB_DEBUG_TRACE", Required: false},
				},
				BaseURL: schema.BaseURLDef{FromEnv: "GITHUB_BASE_URL"},
			},
		},
	}
	action := &schema.ActionFile{
		Name: "list-repo-issues",
		Request: schema.RequestDef{
			Method: "GET",
			Path:   "/repos/{arg.owner}/{arg.repo}/issues",
			Query: []schema.QueryParam{
				{Name: "state", Value: "{arg.state}"},
				{Name: "per_page", Value: 50},
				{Name: "labels", Value: "{arg.labels}", ArrayFormat: "repeat"},
			},
			Headers: []schema.Header{
				{Name: "X-GitHub-Api-Version", Value: "2022-11-28"},
				{Name: "X-Debug-Trace", Value: "{env.GITHUB_DEBUG_TRACE}"},
			},
		},
		Args: []schema.ArgDef{
			{Name: "owner", Type: "string", Required: true},
			{Name: "repo", Type: "string", Required: true},
			{Name: "state", Type: "string", Required: false},
			{Name: "labels", Type: "array", Required: false},
		},
	}
	out, err := RenderRequestBuilder(cli, action)
	if err != nil {
		t.Fatalf("RenderRequestBuilder error: %v", err)
	}
	for _, part := range []string{
		`/repos/{arg.owner}/{arg.repo}/issues`,
		`query.Add("state"`,
		`query.Add("per_page", "50")`,
		`for _, v := range parsed.Labels`,
		`req.Header.Set("X-GitHub-Api-Version", "2022-11-28")`,
		`if v, ok := envs["GITHUB_DEBUG_TRACE"]`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("rendered output missing %q", part)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_request_builder.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}
