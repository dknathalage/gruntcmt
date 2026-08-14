package main

import (
	"bytes"
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

func TestRunEndToEndBarePlan(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "net", "--name", "networking/s3"}, strings.NewReader(p), &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	s := out.String()
	if !strings.HasPrefix(s, "<!-- gruntcmt:scope=net -->") {
		t.Errorf("missing marker: %q", s)
	}
	if !strings.Contains(s, "1 add") {
		t.Errorf("missing add count: %q", s)
	}
}

func TestRunEmptyStdinFails(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run(nil, strings.NewReader(""), &out, &errBuf); code == 0 {
		t.Fatal("expected non-zero exit for empty stdin")
	}
}

func TestRunDetailFlagOverridesToSummary(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["update"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "x", "--name", "a", "--detail", "summary"}, strings.NewReader(p), &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if strings.Contains(out.String(), "```diff") {
		t.Errorf("summary detail should emit no diff block:\n%s", out.String())
	}
}

func TestRunInvalidDetailFlagFails(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "x", "--name", "a", "--detail", "bogus"}, strings.NewReader(p), &out, &errBuf)
	if code == 0 {
		t.Fatalf("expected non-zero exit for invalid --detail")
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
