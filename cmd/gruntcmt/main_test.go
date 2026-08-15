package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"--version"}, strings.NewReader(""), &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "gruntcmt") {
		t.Fatalf("stdout = %q, want it to contain %q", out.String(), "gruntcmt")
	}
}

func TestResolveVersionPrefersLdflags(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = "v9.9.9"
	if got := resolveVersion(); got != "v9.9.9" {
		t.Fatalf("resolveVersion() = %q, want v9.9.9", got)
	}
}

func TestResolveVersionDevFallbackNonEmpty(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = "dev"
	// Under `go test`, build info Main.Version is typically "" or "(devel)",
	// so this exercises the fallback path and must never return empty.
	if got := resolveVersion(); got == "" {
		t.Fatal("resolveVersion() returned empty string")
	}
}

func TestRunEndToEndBarePlanRuleset(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "net", "--name", "networking/s3"}, strings.NewReader(p), &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.HasPrefix(out.String(), "<!-- gruntcmt:scope=net -->") {
		t.Errorf("missing marker: %q", out.String())
	}
}

func TestRunRejectsRemovedDetailFlag(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[]}`
	var out, errBuf bytes.Buffer
	if code := run([]string{"--scope", "x", "--detail", "attribute"}, strings.NewReader(p), &out, &errBuf); code == 0 {
		t.Fatal("expected non-zero: --detail was removed")
	}
}

func TestRunMultiReportStdout(t *testing.T) {
	dir := t.TempDir()
	rulesetPath := dir + "/gruntcmt.yaml"
	os.WriteFile(rulesetPath, []byte(`
rules:
  - path: "**"
    delete: resource
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    delete: resource
`), 0o644)
	in := `{"name":"prod/networking","plan":{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"a","change":{"actions":["create"]}}]}}` + "\n" +
		`{"name":"prod/security/iam","plan":{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"b","change":{"actions":["delete"]}}]}}` + "\n"
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "infra", "--ruleset", rulesetPath}, strings.NewReader(in), &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "<!-- gruntcmt:scope=infra -->") ||
		!strings.Contains(out.String(), "<!-- gruntcmt:scope=security -->") {
		t.Errorf("expected both main and dedicated markers:\n%s", out.String())
	}
}

func TestRunEmptyStdinFails(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run(nil, strings.NewReader(""), &out, &errBuf); code == 0 {
		t.Fatal("expected non-zero exit for empty stdin")
	}
}

func TestRunInvalidInputModeFails(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "x", "--name", "a", "--input", "bogus"}, strings.NewReader(p), &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected non-zero exit for invalid --input")
	}
}

func TestRunOutGhWithoutTokenFails(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "x", "--name", "a", "--out", "gh"}, strings.NewReader(p), &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected non-zero exit without token; stderr=%s", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("--out gh should not write markdown to stdout, got %q", out.String())
	}
}

func TestRunInvalidOutFails(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	if code := run([]string{"--scope", "x", "--name", "a", "--out", "bogus"}, strings.NewReader(p), &out, &errBuf); code == 0 {
		t.Fatal("expected non-zero exit for invalid --out")
	}
}

func TestRunOutStdoutDefaultUnchanged(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "x", "--name", "a"}, strings.NewReader(p), &out, &errBuf)
	if code != 0 || !strings.HasPrefix(out.String(), "<!-- gruntcmt:scope=x -->") {
		t.Fatalf("default stdout path changed: code=%d out=%q", code, out.String())
	}
}
