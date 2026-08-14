package plan

import (
	"os"
	"testing"
)

func load(t *testing.T, name string) Unit {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	u, err := ParsePlan(name, raw)
	if err != nil {
		t.Fatalf("ParsePlan(%s): %v", name, err)
	}
	return u
}

func TestParseCreate(t *testing.T) {
	u := load(t, "create.json")
	if u.TerraformVersion != "1.9.5" {
		t.Errorf("tf version = %q", u.TerraformVersion)
	}
	if len(u.Changes) != 1 || u.Changes[0].Action != ActionCreate {
		t.Fatalf("changes = %+v", u.Changes)
	}
	if u.Counts != (Counts{Add: 1}) {
		t.Errorf("counts = %+v, want {Add:1}", u.Counts)
	}
}

func TestParseReplaceCountsAsAddAndDestroy(t *testing.T) {
	u := load(t, "replace.json")
	want := Counts{Add: 1, Destroy: 1, Replace: 1, NoOp: 1}
	if u.Counts != want {
		t.Errorf("counts = %+v, want %+v", u.Counts, want)
	}
	if u.Changes[0].Action != ActionReplace {
		t.Errorf("action = %v, want Replace", u.Changes[0].Action)
	}
}

func TestParseRejectsNonPlan(t *testing.T) {
	if _, err := ParsePlan("x", []byte(`{"name":"vpc"}`)); err == nil {
		t.Fatal("expected error for missing format_version")
	}
}

func TestSeverityOrdering(t *testing.T) {
	if !(ActionDelete.Severity() > ActionUpdate.Severity() &&
		ActionUpdate.Severity() > ActionCreate.Severity() &&
		ActionCreate.Severity() > ActionNoOp.Severity()) {
		t.Fatal("severity ordering wrong")
	}
	if ActionReplace.Severity() != ActionDelete.Severity() {
		t.Fatal("replace should rank equal to delete")
	}
}
