package render

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/gruntcmt/internal/analyze"
	"github.com/dknathalage/gruntcmt/internal/plan"
)

var update = flag.Bool("update", false, "update golden files")

// sampleReport creates a report where the single ResourceChange has the given
// per-change Detail fidelity (not unit-level).
func sampleReport(detail plan.Fidelity) analyze.Report {
	u := plan.Unit{Name: "production/db", TerraformVersion: "1.9.5",
		Counts: plan.Counts{Add: 1, Destroy: 1, Replace: 1},
		Changes: []plan.ResourceChange{{
			Address: "aws_db_instance.primary", Action: plan.ActionReplace,
			Detail: detail,
			Attributes: []plan.AttributeChange{
				{Path: "engine_version", Before: "14.7", After: "15.4", Kind: plan.AttrUpdate, ForcesNew: true},
			},
		}},
	}
	return analyze.Report{
		Scope: "infra", Title: "Terragrunt plan", TerraformVersion: "1.9.5",
		Groups: []analyze.Group{{Key: "production", Units: []plan.Unit{u}, Counts: u.Counts, Severity: 5}},
		Totals: u.Counts, Severity: 5,
	}
}

func TestRenderGolden(t *testing.T) {
	cases := map[string]plan.Fidelity{
		"summary":   plan.FidelitySummary,
		"resource":  plan.FidelityResource,
		"attribute": plan.FidelityAttribute,
	}
	for name, detail := range cases {
		t.Run(name, func(t *testing.T) {
			got := Render(sampleReport(detail))
			golden := filepath.Join("testdata", name+".golden")
			if *update {
				os.MkdirAll("testdata", 0o755)
				os.WriteFile(golden, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden: %v (run -update first)", err)
			}
			if got != string(want) {
				t.Errorf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func TestRenderMarkerFirstLine(t *testing.T) {
	out := Render(sampleReport(plan.FidelityResource))
	if want := "<!-- gruntcmt:scope=infra -->"; out[:len(want)] != want {
		t.Fatalf("first line = %q", out[:len(want)])
	}
}

// extrasReport exercises: LoadErrors, AttrAdd/AttrRemove glyphs, and a flat group (empty Key "").
// No emoji override, no HideUnchanged — those features are gone.
func extrasReport() analyze.Report {
	u := plan.Unit{
		Name: "db", TerraformVersion: "1.10.0",
		Counts: plan.Counts{Add: 1, Change: 1},
		Changes: []plan.ResourceChange{{
			Address:   "aws_db_instance.primary",
			Action:    plan.ActionUpdate,
			Detail:    plan.FidelityAttribute,
			Unchanged: 3,
			Attributes: []plan.AttributeChange{
				{Path: "tags", After: `{env = "prod"}`, Kind: plan.AttrAdd},
				{Path: "old_param", Before: "legacy", Kind: plan.AttrRemove},
				{Path: "engine_version", Before: "14.7", After: "15.4", Kind: plan.AttrUpdate},
			},
		}},
	}
	return analyze.Report{
		Scope: "extras", Title: "Extras plan", TerraformVersion: "1.10.0",
		Groups: []analyze.Group{{Key: "", Units: []plan.Unit{u}, Counts: u.Counts, Severity: 3}},
		Totals: u.Counts, Severity: 3,
		LoadErrors: []plan.LoadError{
			{Name: "broken/unit", Message: "failed to parse plan JSON"},
		},
	}
}

func TestRenderGoldenExtras(t *testing.T) {
	got := Render(extrasReport())
	golden := filepath.Join("testdata", "extras.golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(golden, []byte(got), 0o644)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v (run -update first)", err)
	}
	if got != string(want) {
		t.Errorf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// Must NOT have "(N unchanged attributes hidden)" line.
	if strings.Contains(got, "unchanged attributes hidden") {
		t.Errorf("output must not contain 'unchanged attributes hidden':\n%s", got)
	}
}

// noopUnitReport has two units in one group: one with real changes and one
// that is entirely no-op (zero Changes). The no-op unit must render as a
// "no changes" one-liner.
func noopUnitReport() analyze.Report {
	active := plan.Unit{
		Name: "staging/api", TerraformVersion: "1.9.5",
		Counts: plan.Counts{Add: 1},
		Changes: []plan.ResourceChange{{
			Address: "aws_lambda_function.api", Action: plan.ActionCreate,
			Detail: plan.FidelityResource,
		}},
	}
	// A unit with no Changes at all is a true no-op.
	noop := plan.Unit{
		Name:   "staging/idle",
		Counts: plan.Counts{NoOp: 2},
		// No Changes — zero-length slice.
	}
	g := analyze.Group{
		Key:      "staging",
		Units:    []plan.Unit{active, noop},
		Counts:   plan.Counts{Add: 1},
		Severity: 2,
	}
	return analyze.Report{
		Scope: "noop-unit-test", Title: "Terragrunt plan", TerraformVersion: "1.9.5",
		Groups:   []analyze.Group{g},
		Totals:   plan.Counts{Add: 1, NoOp: 2},
		Severity: 2,
	}
}

func TestRenderGoldenNoopUnit(t *testing.T) {
	got := Render(noopUnitReport())
	golden := filepath.Join("testdata", "noop-unit.golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(golden, []byte(got), 0o644)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v (run -update first)", err)
	}
	if got != string(want) {
		t.Errorf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// Explicit guard: no empty diff block must appear in the output.
	if strings.Contains(got, "```diff\n```") {
		t.Errorf("output contains empty diff block:\n%s", got)
	}
}

// TestRenderPerChangeDetailMix verifies per-change Detail filtering:
// a summary-detail create is absent from diff output; an attribute-detail delete is present.
func TestRenderPerChangeDetailMix(t *testing.T) {
	u := plan.Unit{Name: "prod/db", TerraformVersion: "1.9.5",
		Counts: plan.Counts{Add: 1, Destroy: 1},
		Changes: []plan.ResourceChange{
			{Address: "aws_x.new", Action: plan.ActionCreate, Detail: plan.FidelitySummary},
			{Address: "aws_y.old", Action: plan.ActionDelete, Detail: plan.FidelityAttribute,
				Attributes: []plan.AttributeChange{{Path: "name", Before: "n", Kind: plan.AttrRemove}}},
		}}
	r := analyze.Report{Scope: "s", Title: "T", TerraformVersion: "1.9.5", GroupBy: 1,
		Groups: []analyze.Group{{Key: "prod", Units: []plan.Unit{u}, Counts: u.Counts, Severity: 5}},
		Totals: u.Counts, Severity: 5}
	out := Render(r)
	if strings.Contains(out, "aws_x.new") {
		t.Error("summary-detail create should not be listed")
	}
	if !strings.Contains(out, "- aws_y.old") {
		t.Errorf("attribute-detail delete missing:\n%s", out)
	}
}

func TestMarker(t *testing.T) {
	if got := Marker("infra"); got != "<!-- gruntcmt:scope=infra -->" {
		t.Fatalf("Marker(infra) = %q", got)
	}
	if got := Marker(""); got != "<!-- gruntcmt:scope= -->" {
		t.Fatalf("Marker() = %q", got)
	}
}

func TestGroupLabelEmptyKeyRendersAll(t *testing.T) {
	if groupLabel("") != "(all)" || groupLabel("prod") != "prod" {
		t.Fatalf("groupLabel: empty=%q prod=%q", groupLabel(""), groupLabel("prod"))
	}
	u := plan.Unit{Name: "x", Changes: []plan.ResourceChange{{Address: "a", Action: plan.ActionCreate, Detail: plan.FidelityResource}}, Counts: plan.Counts{Add: 1}}
	r := analyze.Report{Scope: "s", Title: "T", GroupBy: 0,
		Groups: []analyze.Group{{Key: "", Units: []plan.Unit{u}, Counts: u.Counts, Severity: 2}},
		Totals: u.Counts, Severity: 2}
	out := Render(r)
	if strings.Contains(out, "<code></code>") {
		t.Errorf("empty group key still renders <code></code>:\n%s", out)
	}
	if !strings.Contains(out, "<code>(all)</code> — 1 units") {
		t.Errorf("group header should use (all):\n%s", out)
	}
}
