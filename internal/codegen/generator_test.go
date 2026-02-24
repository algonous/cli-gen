package codegen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/autonous/cli-gen/internal/schema"
)

func mustParseGo(t *testing.T, label, src string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), label, src, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go (%s): %v\n--- source ---\n%s", label, err, src)
	}
}

func TestRenderMain(t *testing.T) {
	set := &schema.SchemaSet{
		CLI: &schema.CliFile{Cli: schema.CliDef{Name: "github", Description: "GitHub REST API CLI"}},
		Actions: []*schema.ActionFile{
			{Name: "list-repo-issues", Impl: "generated"},
			{Name: "create-issue", Impl: "generated"},
			{Name: "create-or-update-file", Impl: "custom"},
		},
	}

	out, err := RenderMain(set)
	if err != nil {
		t.Fatalf("RenderMain error: %v", err)
	}

	for _, part := range []string{
		"list-repo-issues", "create-issue", "create-or-update-file", "skill",
		`fs.String("log-file"`,
		"github/generated",
		"github/custom",
		"github/internal",
		"registry",
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("rendered output missing %q\n--- source ---\n%s", part, out)
		}
	}

	mustParseGo(t, "generated_main.go", out)
}

func TestRenderMainOnlyGenerated(t *testing.T) {
	set := &schema.SchemaSet{
		CLI: &schema.CliFile{Cli: schema.CliDef{Name: "mycli", Description: "My CLI"}},
		Actions: []*schema.ActionFile{
			{Name: "get-thing", Impl: "generated"},
		},
	}
	out, err := RenderMain(set)
	if err != nil {
		t.Fatalf("RenderMain error: %v", err)
	}
	if strings.Contains(out, "mycli/custom") {
		t.Fatalf("should not import custom when no custom actions: %s", out)
	}
	mustParseGo(t, "generated_main_only_gen.go", out)
}

func TestRenderInternalRuntime(t *testing.T) {
	cli := &schema.CliFile{
		Cli: schema.CliDef{
			Runtime: schema.RuntimeDef{
				Env: []schema.EnvEntry{
					{Name: "GITHUB_TOKEN", Secret: true},
					{Name: "GITHUB_BASE_URL", Secret: false},
				},
			},
		},
	}
	out, err := RenderInternalRuntime(cli)
	if err != nil {
		t.Fatalf("RenderInternalRuntime error: %v", err)
	}
	for _, part := range []string{
		`package internal`,
		`var LogFilePath string`,
		`json:"ok"`,
		`json:"body_text,omitempty"`,
		`BodyText *string`,
		`func LoadEnvs(`,
		`func CallAndWriteOutput(`,
		`func BuildEnvelope(`,
		`func WriteOutput(`,
		`func AppendLogLine(`,
		`func PlaceholderExit(`,
		`case "GITHUB_TOKEN":`,
		`os.OpenFile`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("RenderInternalRuntime missing %q\n--- source ---\n%s", part, out)
		}
	}
	if strings.Contains(out, `case "GITHUB_BASE_URL"`) {
		t.Fatalf("non-secret env should not be in IsSecretEnv switch: %s", out)
	}
	mustParseGo(t, "generated_internal_runtime.go", out)
}

func TestRenderGeneratedAction(t *testing.T) {
	cli := &schema.CliFile{
		Cli: schema.CliDef{
			Runtime: schema.RuntimeDef{
				Env: []schema.EnvEntry{
					{Name: "GITHUB_BASE_URL", Required: true},
					{Name: "GITHUB_DEBUG_TRACE", Required: false},
					{Name: "GITHUB_TOKEN", Required: true, Secret: true},
				},
				BaseURL: schema.BaseURLDef{FromEnv: "GITHUB_BASE_URL"},
				Auth:    &schema.AuthDef{Header: "Authorization", Template: "Bearer {env.GITHUB_TOKEN}"},
			},
		},
	}
	action := &schema.ActionFile{
		Name: "list-repo-issues",
		Impl: "generated",
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
			{Name: "owner", Type: "string", Required: true, Help: "owner"},
			{Name: "repo", Type: "string", Required: true, Help: "repo"},
			{Name: "state", Type: "string", Required: false, Help: "state"},
			{Name: "labels", Type: "array", Required: false, Help: "labels"},
		},
		Response: schema.ResponseDef{SuccessStatus: []int{200}},
	}

	out, err := RenderGeneratedAction("github", cli, action)
	if err != nil {
		t.Fatalf("RenderGeneratedAction error: %v", err)
	}
	for _, part := range []string{
		`package generated`,
		`func ListRepoIssues(args []string)`,
		`/repos/{arg.owner}/{arg.repo}/issues`,
		`query.Add("state"`,
		`query.Add("per_page", "50")`,
		`for _, v := range parsed.Labels`,
		`req.Header.Set("X-GitHub-Api-Version", "2022-11-28")`,
		`if v, ok := envs["GITHUB_DEBUG_TRACE"]`,
		`req.Header.Set("Authorization"`,
		`internal.LoadEnvs(`,
		`internal.CallAndWriteOutput(`,
		`short flags are not supported`,
		`fs.Visit(func(f *flag.Flag)`,
		`fs.Usage = func()`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("RenderGeneratedAction missing %q\n--- source ---\n%s", part, out)
		}
	}
	mustParseGo(t, "generated_list_repo_issues.go", out)
}

func TestRenderGeneratedActionWithBody(t *testing.T) {
	cli := &schema.CliFile{
		Cli: schema.CliDef{
			Runtime: schema.RuntimeDef{
				Env:     []schema.EnvEntry{{Name: "GITHUB_BASE_URL", Required: true}},
				BaseURL: schema.BaseURLDef{FromEnv: "GITHUB_BASE_URL"},
			},
		},
	}
	action := &schema.ActionFile{
		Name: "create-issue",
		Impl: "generated",
		Request: schema.RequestDef{
			Method: "POST",
			Path:   "/repos/{arg.owner}/{arg.repo}/issues",
			Body: &schema.BodyDef{
				Mode: "template",
				Template: map[string]any{
					"title":     "{arg.title}",
					"body":      "{arg.body}",
					"assignees": "{arg.assignees}",
					"labels":    "{arg.labels}",
				},
			},
		},
		Args: []schema.ArgDef{
			{Name: "owner", Type: "string", Required: true, Help: "owner"},
			{Name: "repo", Type: "string", Required: true, Help: "repo"},
			{Name: "title", Type: "string", Required: true, Help: "title"},
			{Name: "body", Type: "string", Required: false, Help: "body"},
			{Name: "assignees", Type: "array", Required: false, Help: "assignees"},
			{Name: "labels", Type: "array", Required: false, Help: "labels"},
		},
		Response: schema.ResponseDef{SuccessStatus: []int{201}},
	}

	out, err := RenderGeneratedAction("github", cli, action)
	if err != nil {
		t.Fatalf("RenderGeneratedAction (body) error: %v", err)
	}
	for _, part := range []string{
		`func buildCreateIssueBody(`,
		`payload["title"]`,
		`payload["body"]`,
		`payload["assignees"]`,
		`payload["labels"]`,
		`json.Marshal(payload)`,
		`bodyBytes, err := buildCreateIssueBody(parsed)`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("RenderGeneratedAction (body) missing %q\n--- source ---\n%s", part, out)
		}
	}
	mustParseGo(t, "generated_create_issue.go", out)
}

func TestRenderGeneratedActionRawJSON(t *testing.T) {
	cli := &schema.CliFile{
		Cli: schema.CliDef{
			Runtime: schema.RuntimeDef{
				Env:     []schema.EnvEntry{{Name: "GITHUB_BASE_URL", Required: true}},
				BaseURL: schema.BaseURLDef{FromEnv: "GITHUB_BASE_URL"},
			},
		},
	}
	action := &schema.ActionFile{
		Name: "create-or-update-file",
		Impl: "generated",
		Request: schema.RequestDef{
			Method: "PUT",
			Path:   "/repos/{arg.owner}/{arg.repo}/contents/{arg.path}",
			Body:   &schema.BodyDef{Mode: "raw_json_arg", Arg: "payload-json"},
		},
		Args: []schema.ArgDef{
			{Name: "owner", Type: "string", Required: true, Help: "owner"},
			{Name: "repo", Type: "string", Required: true, Help: "repo"},
			{Name: "path", Type: "string", Required: true, Help: "path"},
			{Name: "payload-json", Type: "string", Required: true, Help: "payload"},
		},
		Response: schema.ResponseDef{SuccessStatus: []int{200, 201}},
	}

	out, err := RenderGeneratedAction("github", cli, action)
	if err != nil {
		t.Fatalf("RenderGeneratedAction (raw_json) error: %v", err)
	}
	for _, part := range []string{`parsed.PayloadJson`, `json.Valid`, `[]byte(raw)`} {
		if !strings.Contains(out, part) {
			t.Fatalf("RenderGeneratedAction (raw_json) missing %q\n--- source ---\n%s", part, out)
		}
	}
	mustParseGo(t, "generated_raw_json.go", out)
}

func TestRenderCustomAction(t *testing.T) {
	action := &schema.ActionFile{Name: "create-or-update-file", Impl: "custom"}
	out, err := RenderCustomAction("github", action)
	if err != nil {
		t.Fatalf("RenderCustomAction error: %v", err)
	}
	for _, part := range []string{
		`package custom`,
		`func CreateOrUpdateFile(args []string)`,
		`internal.PlaceholderExit("create-or-update-file")`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("RenderCustomAction missing %q\n--- source ---\n%s", part, out)
		}
	}
	mustParseGo(t, "generated_custom_action.go", out)
}

func TestRenderSkillHandler(t *testing.T) {
	set := &schema.SchemaSet{
		CLI: &schema.CliFile{Cli: schema.CliDef{Name: "github"}},
		Actions: []*schema.ActionFile{
			{
				Name:        "list-repo-issues",
				Description: "List issues",
				Request:     schema.RequestDef{Method: "GET", Path: "/repos/{arg.owner}/{arg.repo}/issues"},
				Args:        []schema.ArgDef{{Name: "owner", Type: "string", Help: "owner"}},
			},
			{
				Name:        "create-issue",
				Description: "Create issue",
				Request:     schema.RequestDef{Method: "POST", Path: "/repos/{arg.owner}/{arg.repo}/issues"},
				Args:        []schema.ArgDef{{Name: "repo", Type: "string", Help: "repo"}},
			},
			{
				Name:        "create-or-update-file",
				Description: "Create file",
				Request:     schema.RequestDef{Method: "PUT", Path: "/repos/{arg.owner}/{arg.repo}/contents/{arg.path}"},
			},
		},
	}
	out, err := RenderSkillHandler(set)
	if err != nil {
		t.Fatalf("RenderSkillHandler error: %v", err)
	}
	for _, part := range []string{
		"list-repo-issues", "create-issue", "create-or-update-file", "skill_written",
		"SKILL.md", "name: github-skill",
		"## Purpose", "## Workflow", "## Actions",
		"### list-repo-issues", "### Arguments", "### Examples",
		"- Endpoint: `GET /repos/{arg.owner}/{arg.repo}/issues`",
		"## Output Contract",
		"github/internal",
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("RenderSkillHandler missing %q\n--- source ---\n%s", part, out)
		}
	}
	mustParseGo(t, "generated_skill.go", out)
}
