package codegen

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"net/http"
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
	if !strings.Contains(out, `flag.String("log-file"`) {
		t.Fatalf("rendered main missing --log-file flag")
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

func TestRenderBodyBuilderTemplateMode(t *testing.T) {
	action := &schema.ActionFile{
		Name: "create-issue",
		Request: schema.RequestDef{
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
			{Name: "title", Type: "string", Required: true},
			{Name: "body", Type: "string", Required: false},
			{Name: "assignees", Type: "array", Required: false},
			{Name: "labels", Type: "array", Required: false},
		},
	}
	out, err := RenderBodyBuilder(action)
	if err != nil {
		t.Fatalf("RenderBodyBuilder error: %v", err)
	}
	for _, part := range []string{`payload["title"]`, `payload["body"]`, `payload["assignees"]`, `payload["labels"]`} {
		if !strings.Contains(out, part) {
			t.Fatalf("rendered output missing %q", part)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_body_builder.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}

func TestRenderBodyBuilderRawJSONMode(t *testing.T) {
	action := &schema.ActionFile{
		Name: "create-or-update-file",
		Request: schema.RequestDef{
			Body: &schema.BodyDef{Mode: "raw_json_arg", Arg: "payload-json"},
		},
		Args: []schema.ArgDef{{Name: "payload-json", Type: "string", Required: true}},
	}
	out, err := RenderBodyBuilder(action)
	if err != nil {
		t.Fatalf("RenderBodyBuilder error: %v", err)
	}
	for _, part := range []string{`parsed.PayloadJson`, `json.Valid`, `[]byte(raw)`} {
		if !strings.Contains(out, part) {
			t.Fatalf("rendered output missing %q", part)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_body_builder_raw.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}

func TestRenderExecutor(t *testing.T) {
	action := &schema.ActionFile{Name: "list-repo-issues", Response: schema.ResponseDef{SuccessStatus: []int{200, 201}}}
	out, err := RenderExecutor(action)
	if err != nil {
		t.Fatalf("RenderExecutor error: %v", err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_executor.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}

func TestBuildEnvelope(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		ok     bool
		hasB   bool
		hasTxt bool
	}{
		{name: "success json", status: 200, body: `{"id":1}`, ok: true, hasB: true},
		{name: "error json", status: 404, body: `{"message":"not found"}`, ok: false, hasB: true},
		{name: "error text", status: 500, body: `internal error`, ok: false, hasTxt: true},
		{name: "success empty", status: 204, body: ``, ok: true, hasTxt: true},
		{name: "success text", status: 200, body: `OK`, ok: true, hasTxt: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tc.status}
			env, err := BuildEnvelope([]int{200, 204}, resp, []byte(tc.body))
			if err != nil {
				t.Fatalf("BuildEnvelope error: %v", err)
			}
			if env.OK != tc.ok {
				t.Fatalf("ok=%v want=%v", env.OK, tc.ok)
			}
			if tc.hasB && env.Body == nil {
				t.Fatal("expected body")
			}
			if tc.hasTxt && env.BodyText == "" && tc.body != "" {
				t.Fatal("expected body_text")
			}

			encoded, err := json.Marshal(env)
			if err != nil {
				t.Fatalf("marshal envelope: %v", err)
			}
			if !bytes.Contains(encoded, []byte(`"status"`)) {
				t.Fatalf("invalid envelope json: %s", encoded)
			}
		})
	}
}

func TestRenderCustomDispatcher(t *testing.T) {
	out, err := RenderCustomDispatcher("create-or-update-file")
	if err != nil {
		t.Fatalf("RenderCustomDispatcher error: %v", err)
	}
	if !strings.Contains(out, "customBindingCreateOrUpdateFile") {
		t.Fatalf("dispatcher missing custom binding call: %s", out)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_custom_dispatcher.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}

func TestRenderCustomBindingStub(t *testing.T) {
	out, err := RenderCustomBindingStub("create-or-update-file")
	if err != nil {
		t.Fatalf("RenderCustomBindingStub error: %v", err)
	}
	if !strings.Contains(out, `"status":-2`) || !strings.Contains(out, "PLACEHOLDER: create-or-update-file requires a custom binding") {
		t.Fatalf("stub missing placeholder envelope: %s", out)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_custom_bindings.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}

func TestRenderSecretsHelpers(t *testing.T) {
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
	out, err := RenderSecretsHelpers(cli)
	if err != nil {
		t.Fatalf("RenderSecretsHelpers error: %v", err)
	}
	if !strings.Contains(out, `case "GITHUB_TOKEN"`) {
		t.Fatalf("missing secret env switch case: %s", out)
	}
	if strings.Contains(out, `case "GITHUB_BASE_URL"`) {
		t.Fatalf("non-secret env should not be in switch: %s", out)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_secrets.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}

func TestRenderLogFileHelper(t *testing.T) {
	out, err := RenderLogFileHelper()
	if err != nil {
		t.Fatalf("RenderLogFileHelper error: %v", err)
	}
	if !strings.Contains(out, "os.OpenFile") {
		t.Fatalf("missing os.OpenFile in log helper: %s", out)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated_logfile.go", out, parser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}
}
