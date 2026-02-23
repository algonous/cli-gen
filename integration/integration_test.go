package integration

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, dir string, env []string, name string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), stdout.String(), stderr.String()
	}
	t.Fatalf("run command %s %v: %v", name, args, err)
	return -1, "", ""
}

func TestIntegration(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(t.TempDir(), "go-cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goEnv := []string{"GOCACHE=" + cacheDir}

	t.Run("T23a schema validation", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "gen-github")
		code, _, _ := runCmd(t, repoRoot, goEnv, "go", "run", "./cmd/cli-gen", "./integration/testdata/github-schema", "--out", outDir)
		if code != 0 {
			t.Fatalf("expected success, got code=%d", code)
		}

		code, _, stderr := runCmd(t, repoRoot, goEnv, "go", "run", "./cmd/cli-gen", "./integration/testdata/bad-duplicate-schema", "--out", filepath.Join(t.TempDir(), "x"))
		if code == 0 || !strings.Contains(strings.ToLower(stderr), "duplicate") {
			t.Fatalf("expected duplicate error, code=%d stderr=%s", code, stderr)
		}

		code, _, stderr = runCmd(t, repoRoot, goEnv, "go", "run", "./cmd/cli-gen", "./integration/testdata/bad-unknown-field", "--out", filepath.Join(t.TempDir(), "x2"))
		if code == 0 || !strings.Contains(strings.ToLower(stderr), "unknown") {
			t.Fatalf("expected unknown field error, code=%d stderr=%s", code, stderr)
		}
	})

	genDir := filepath.Join(t.TempDir(), "generated")
	code, _, stderr := runCmd(t, repoRoot, goEnv, "go", "run", "./cmd/cli-gen", "./integration/testdata/github-schema", "--out", genDir)
	if code != 0 {
		t.Fatalf("generate failed: %s", stderr)
	}

	t.Run("T23b generated compile", func(t *testing.T) {
		code, _, stderr := runCmd(t, genDir, goEnv, "go", "build", "-o", "github", ".")
		if code != 0 {
			t.Fatalf("go build failed: %s", stderr)
		}
	})

	binary := filepath.Join(genDir, "github")
	if _, err := os.Stat(binary); err != nil {
		t.Fatalf("binary missing: %v", err)
	}

	t.Run("T23c envelope", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Skipf("sandbox does not allow local listeners: %v", err)
		}
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Query().Get("state") {
			case "ok":
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"items":[1]}`))
			case "jsonerr":
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"message":"not found"}`))
			default:
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("boom"))
			}
		}))
		server.Listener = ln
		server.Start()
		defer server.Close()

		env := append(goEnv, "GITHUB_BASE_URL="+server.URL)
		code, stdout, _ := runCmd(t, genDir, env, binary, "list-repo-issues", "--owner", "o", "--repo", "r", "--state", "ok")
		if code != 0 {
			t.Fatalf("expected success, code=%d stdout=%s", code, stdout)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &m); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if m["ok"] != true {
			t.Fatalf("expected ok=true, got %v", m["ok"])
		}

		code, stdout, _ = runCmd(t, genDir, env, binary, "list-repo-issues", "--owner", "o", "--repo", "r", "--state", "jsonerr")
		if code == 0 || !strings.Contains(stdout, `"status":404`) {
			t.Fatalf("expected 404 envelope, code=%d stdout=%s", code, stdout)
		}

		code, stdout, _ = runCmd(t, genDir, env, binary, "list-repo-issues", "--owner", "o", "--repo", "r")
		if code == 0 || !strings.Contains(stdout, `"body_text":"boom"`) {
			t.Fatalf("expected non-json envelope, code=%d stdout=%s", code, stdout)
		}
	})

	t.Run("T23d custom stub", func(t *testing.T) {
		code, stdout, _ := runCmd(t, genDir, goEnv, binary, "create-or-update-file", "--owner", "o", "--repo", "r", "--path", "p", "--payload-json", "{}")
		if code == 0 || !strings.Contains(stdout, `"status":-2`) || !strings.Contains(stdout, "PLACEHOLDER") {
			t.Fatalf("expected custom placeholder, code=%d stdout=%s", code, stdout)
		}
	})

	t.Run("T23e skill", func(t *testing.T) {
		code, stdout, _ := runCmd(t, genDir, goEnv, binary, "skill")
		if code != 0 || !strings.Contains(stdout, "list-repo-issues") || !strings.Contains(stdout, "create-issue") {
			t.Fatalf("unexpected skill output, code=%d stdout=%s", code, stdout)
		}

		outPath := filepath.Join(t.TempDir(), "sk")
		code, stdout, _ = runCmd(t, genDir, goEnv, binary, "skill", "--output", outPath)
		if code != 0 {
			t.Fatalf("skill --output failed: %s", stdout)
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &event); err != nil {
			t.Fatalf("invalid event json: %v", err)
		}
		if event["event"] != "skill_written" {
			t.Fatalf("unexpected event: %v", event)
		}
		path, ok := event["path"].(string)
		if !ok || !strings.HasSuffix(path, "SKILL.md") {
			t.Fatalf("unexpected skill path: %v", event["path"])
		}
		contentPath := filepath.Join(outPath, "SKILL.md")
		content, err := os.ReadFile(contentPath)
		if err != nil {
			t.Fatalf("expected SKILL.md to be written: %v", err)
		}
		text := string(content)
		if !strings.Contains(text, "# Skill") ||
			!strings.Contains(text, "name: github-api-workflow") ||
			!strings.Contains(text, "## Workflow") ||
			!strings.Contains(text, "### list-repo-issues") ||
			!strings.Contains(text, "### Examples") ||
			!strings.Contains(text, "github list-repo-issues --owner 'algonous' --repo 'cli-gen'") {
			t.Fatalf("unexpected SKILL.md content: %s", text)
		}
	})

	t.Run("T23f log-file", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Skipf("sandbox does not allow local listeners: %v", err)
		}
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		}))
		server.Listener = ln
		server.Start()
		defer server.Close()
		logPath := filepath.Join(t.TempDir(), "test.log")
		env := append(goEnv, "GITHUB_BASE_URL="+server.URL)

		code, _, _ := runCmd(t, genDir, env, binary, "--log-file", logPath, "list-repo-issues", "--owner", "o", "--repo", "r", "--state", "ok")
		if code != 0 {
			t.Fatalf("first run failed, code=%d", code)
		}
		code, _, _ = runCmd(t, genDir, env, binary, "--log-file", logPath, "list-repo-issues", "--owner", "o", "--repo", "r", "--state", "ok")
		if code != 0 {
			t.Fatalf("second run failed, code=%d", code)
		}
		content, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read log file: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(string(content)), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(content))
		}
	})
}
