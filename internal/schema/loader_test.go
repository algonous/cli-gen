package schema

import (
	"os"
	"path/filepath"
	"testing"
)

const validCLIYAML = `schema_version: v1
cli:
  name: github
  description: GitHub REST API CLI
  runtime:
    env:
      - name: GITHUB_BASE_URL
        required: true
      - name: GITHUB_TOKEN
        required: true
        secret: true
    base_url:
      from_env: GITHUB_BASE_URL
    auth:
      header: Authorization
      template: Bearer {env.GITHUB_TOKEN}
`

const validActionYAML = `name: list-repo-issues
impl: generated
description: List issues
request:
  method: GET
  path: /repos/{arg.owner}/{arg.repo}/issues
args:
  - name: owner
    type: string
    required: true
    help: owner
  - name: repo
    type: string
    required: true
    help: repo
response:
  success_status: [200]
`

func TestLoadCLIFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yaml")
	if err := os.WriteFile(path, []byte(validCLIYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cli, err := LoadCLIFile(path)
	if err != nil {
		t.Fatalf("LoadCLIFile error: %v", err)
	}
	if cli.Cli.Name != "github" {
		t.Fatalf("got %q", cli.Cli.Name)
	}
}

func TestLoadCLIFileUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yaml")
	bad := validCLIYAML + "extra_field: x\n"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCLIFile(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadActionFileUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "action.yaml")
	bad := validActionYAML + "extra_field: x\n"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadActionFile(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadSchemaDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cli.yaml"), []byte(validCLIYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	actionsDir := filepath.Join(dir, "actions")
	if err := os.Mkdir(actionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(actionsDir, "a.yaml"), []byte(validActionYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(actionsDir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	set, err := LoadSchemaDir(dir)
	if err != nil {
		t.Fatalf("LoadSchemaDir error: %v", err)
	}
	if len(set.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(set.Actions))
	}
}

func TestLoadSchemaDirMissingCLI(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadSchemaDir(dir); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadSchemaDirMissingActions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cli.yaml"), []byte(validCLIYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := LoadSchemaDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(set.Actions) != 0 {
		t.Fatalf("expected 0 actions, got %d", len(set.Actions))
	}
}

func TestCheckDuplicateActions(t *testing.T) {
	set := &SchemaSet{Actions: []*ActionFile{{Name: "a"}, {Name: "a"}}}
	if err := CheckDuplicateActions(set); err == nil {
		t.Fatal("expected duplicate error")
	}
}
