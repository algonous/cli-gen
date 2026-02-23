package main

import (
	"os"
	"path/filepath"
	"testing"
)

const cliYAML = `schema_version: v1
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

const actionYAML = `name: list-repo-issues
impl: generated
description: list
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

func TestRunGenerator(t *testing.T) {
	schemaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(schemaDir, "cli.yaml"), []byte(cliYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	actionsDir := filepath.Join(schemaDir, "actions")
	if err := os.Mkdir(actionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(actionsDir, "list-repo-issues.yaml"), []byte(actionYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	if err := RunGenerator(schemaDir, outDir); err != nil {
		t.Fatalf("RunGenerator error: %v", err)
	}
	for _, f := range []string{"main.go", "go.mod"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Fatalf("missing generated file %s: %v", f, err)
		}
	}
}

func TestRunGeneratorDuplicateActions(t *testing.T) {
	schemaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(schemaDir, "cli.yaml"), []byte(cliYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	actionsDir := filepath.Join(schemaDir, "actions")
	if err := os.Mkdir(actionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(actionsDir, "a.yaml"), []byte(actionYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(actionsDir, "b.yaml"), []byte(actionYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	if err := RunGenerator(schemaDir, outDir); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(outDir); err == nil {
		t.Fatal("output directory should not be created on validation error")
	}
}
