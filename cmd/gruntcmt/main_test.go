package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const barePlan = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`

func writePlan(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(barePlan), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunVersion(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"--version"}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "gruntcmt") {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestResolveVersionPrefersLdflags(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = "v9.9.9"
	if got := resolveVersion(); got != "v9.9.9" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveVersionDevFallbackNonEmpty(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = "dev"
	if got := resolveVersion(); got == "" {
		t.Fatal("resolveVersion() returned empty string")
	}
}

func TestRunToFile(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/networking/tfplan.json"))
	outFile := filepath.Join(t.TempDir(), "report.md")
	var out, errBuf bytes.Buffer
	code := run([]string{"--out", outFile, dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	body, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "gruntcmt:scope="+filepath.Base(dir)) {
		t.Errorf("missing scope marker:\n%s", body)
	}
}

func TestRunToStdoutDevice(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "/dev/stdout", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.HasPrefix(out.String(), "<!-- gruntcmt:scope=") {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestRunSummary(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "summary", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	body, err := os.ReadFile(summary)
	if err != nil || !strings.Contains(string(body), "gruntcmt:scope=") {
		t.Fatalf("summary body=%q err=%v", body, err)
	}
}

func TestRunSummaryWithoutEnvFails(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "summary", dir}, &out, &errBuf); code == 0 {
		t.Fatal("expected failure without GITHUB_STEP_SUMMARY")
	}
}

func TestRunNoArgsFails(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "/dev/stdout"}, &out, &errBuf); code == 0 {
		t.Fatal("expected failure with no plan paths")
	}
}

func TestRunRejectsRemovedFlag(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	var out, errBuf bytes.Buffer
	if code := run([]string{"--scope", "x", "--out", "/dev/stdout", dir}, &out, &errBuf); code == 0 {
		t.Fatal("expected failure: --scope was removed")
	}
}

func TestRunPrintConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "gruntcmt.yaml")
	if err := os.WriteFile(cfg, []byte("rules:\n  - path: \"**\"\n    delete: attribute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if code := run([]string{"--config", cfg, "--print-config"}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "rules:") {
		t.Fatalf("expected ruleset yaml on stderr, got %q", errBuf.String())
	}
}
