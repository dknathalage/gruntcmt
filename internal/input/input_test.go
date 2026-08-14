package input

import (
	"strings"
	"testing"
)

const barePlan = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`

func TestReadBareByDetection(t *testing.T) {
	units, le, err := Read(strings.NewReader(barePlan), ModeAuto, "vpc")
	if err != nil || len(le) != 0 {
		t.Fatalf("err=%v loadErrs=%v", err, le)
	}
	if len(units) != 1 || units[0].Name != "vpc" {
		t.Fatalf("units = %+v", units)
	}
}

func TestReadWrappedNDJSON(t *testing.T) {
	in := `{"name":"a","plan":` + barePlan + `}` + "\n" +
		`{"name":"b","plan":` + barePlan + `}` + "\n"
	units, le, err := Read(strings.NewReader(in), ModeAuto, "")
	if err != nil || len(le) != 0 {
		t.Fatalf("err=%v le=%v", err, le)
	}
	if len(units) != 2 || units[0].Name != "a" || units[1].Name != "b" {
		t.Fatalf("units = %+v", units)
	}
}

func TestReadWrappedIsolatesBadRecord(t *testing.T) {
	in := `{"name":"good","plan":` + barePlan + `}` + "\n" +
		`{"name":"bad","plan":{"nope":true}}` + "\n"
	units, le, err := Read(strings.NewReader(in), ModeAuto, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(units) != 1 || len(le) != 1 || le[0].Name != "bad" {
		t.Fatalf("units=%+v loadErrs=%+v", units, le)
	}
}

func TestReadWrappedStructuralErrorRecorded(t *testing.T) {
	// Valid wrapped record followed by structurally-broken wrapper
	// (missing closing brace for plan object).
	in := `{"name":"good","plan":` + barePlan + `}` + "\n" +
		`{"name":"broken","plan":{"incomplete":true` + "\n"
	units, le, err := Read(strings.NewReader(in), ModeAuto, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	// Valid unit should be returned
	if len(units) != 1 || units[0].Name != "good" {
		t.Fatalf("units=%+v (expected 1 valid unit 'good')", units)
	}
	// At least one LoadError should be recorded for the structural error
	if len(le) == 0 {
		t.Fatalf("loadErrs=%+v (expected at least 1 error for structural decode failure)", le)
	}
}

func TestReadEmptyStdinErrors(t *testing.T) {
	units, le, err := Read(strings.NewReader("   "), ModeAuto, "x")
	if err == nil {
		t.Fatalf("expected non-nil error for empty stdin, got units=%+v le=%+v", units, le)
	}
}
