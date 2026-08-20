package input

import (
	"os"
	"path/filepath"
	"testing"
)

const readBarePlan = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUnitNameFromTree(t *testing.T) {
	if got := unitName("out", filepath.FromSlash("out/aws/prod/networking/tfplan.json")); got != "aws/prod/networking" {
		t.Errorf("unitName = %q want aws/prod/networking", got)
	}
}

func TestReadPathsDirRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "aws/prod/networking/tfplan.json"), readBarePlan)
	writeFile(t, filepath.Join(dir, "aws/prod/db/tfplan.json"), readBarePlan)
	writeFile(t, filepath.Join(dir, "gcp/staging/tfplan.json"), readBarePlan)
	units, le, err := ReadPaths([]string{dir})
	if err != nil || len(le) != 0 {
		t.Fatalf("err=%v le=%v", err, le)
	}
	names := map[string]bool{}
	for _, u := range units {
		names[u.Name] = true
	}
	for _, want := range []string{"aws/prod/networking", "aws/prod/db", "gcp/staging"} {
		if !names[want] {
			t.Errorf("missing unit %q in %v", want, names)
		}
	}
}

func TestReadPathsSingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "vpc.json")
	writeFile(t, f, readBarePlan)
	units, _, err := ReadPaths([]string{f})
	if err != nil || len(units) != 1 || units[0].Name != filepath.ToSlash(filepath.Join(dir, "vpc")) {
		t.Fatalf("units=%+v err=%v", units, err)
	}
}

func TestReadPathsBadPlanIsolated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "good/tfplan.json"), readBarePlan)
	writeFile(t, filepath.Join(dir, "bad/tfplan.json"), `{"no":"format_version"}`)
	units, le, err := ReadPaths([]string{dir})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(units) != 1 || len(le) != 1 {
		t.Fatalf("units=%+v le=%+v", units, le)
	}
}

func TestReadPathsNoArgsErrors(t *testing.T) {
	if _, _, err := ReadPaths(nil); err == nil {
		t.Fatal("expected error for no paths")
	}
}

func TestReadPathsEmptyDirErrors(t *testing.T) {
	if _, _, err := ReadPaths([]string{t.TempDir()}); err == nil {
		t.Fatal("expected error when no plans found")
	}
}
