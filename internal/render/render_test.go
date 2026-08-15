package render

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/gruntcmt/internal/analyze"
	"github.com/dknathalage/gruntcmt/internal/config"
	"github.com/dknathalage/gruntcmt/internal/plan"
)

var update = flag.Bool("update", false, "update golden files")

func sampleReport(detail plan.Fidelity) analyze.Report {
	u := plan.Unit{Name: "production/db", TerraformVersion: "1.9.5", Detail: detail,
		Counts: plan.Counts{Add: 1, Destroy: 1, Replace: 1},
		Changes: []plan.ResourceChange{{
			Address: "aws_db_instance.primary", Action: plan.ActionReplace,
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
			got := Render(sampleReport(detail), config.Settings{})
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
	out := Render(sampleReport(plan.FidelityResource), config.Settings{})
	if want := "<!-- gruntcmt:scope=infra -->"; out[:len(want)] != want {
		t.Fatalf("first line = %q", out[:len(want)])
	}
}

// extrasReport exercises: LoadErrors, emoji override, HideUnchanged, AttrAdd/AttrRemove glyphs,
// and a flat group (empty Key "").
func extrasReport() analyze.Report {
	u := plan.Unit{
		Name: "db", TerraformVersion: "1.10.0", Detail: plan.FidelityAttribute,
		Counts: plan.Counts{Add: 1, Change: 1},
		Changes: []plan.ResourceChange{{
			Address:   "aws_db_instance.primary",
			Action:    plan.ActionUpdate,
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
	s := config.Settings{}
	s.Render.Emoji = map[string]string{"change": "🟠"}
	s.Render.HideUnchanged = true

	got := Render(extrasReport(), s)
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
}

// foldNoopReport has two groups: one with real changes (severity > 0) and one no-op group (severity == 0).
func foldNoopReport() analyze.Report {
	active := plan.Unit{
		Name: "prod/app", TerraformVersion: "1.9.5", Detail: plan.FidelityResource,
		Counts: plan.Counts{Add: 1},
		Changes: []plan.ResourceChange{{
			Address: "aws_instance.web", Action: plan.ActionCreate,
		}},
	}
	noop := plan.Unit{
		Name: "prod/cache", TerraformVersion: "1.9.5", Detail: plan.FidelityResource,
		Counts: plan.Counts{NoOp: 1},
		Changes: []plan.ResourceChange{{
			Address: "aws_elasticache_cluster.c", Action: plan.ActionNoOp,
		}},
	}
	activeGroup := analyze.Group{
		Key:      "prod/app",
		Units:    []plan.Unit{active},
		Counts:   active.Counts,
		Severity: 2,
	}
	noopGroup := analyze.Group{
		Key:      "prod/cache",
		Units:    []plan.Unit{noop},
		Counts:   noop.Counts,
		Severity: 0,
	}
	return analyze.Report{
		Scope: "fold-test", Title: "Terragrunt plan", TerraformVersion: "1.9.5",
		Groups:   []analyze.Group{activeGroup, noopGroup},
		Totals:   plan.Counts{Add: 1, NoOp: 1},
		Severity: 2,
	}
}

// noopUnitReport has two units in one group: one with real changes and one
// that is entirely no-op.  FoldNoop is OFF (the default), so the no-op unit
// must render as a "no changes" one-liner rather than an empty diff block.
func noopUnitReport() analyze.Report {
	active := plan.Unit{
		Name: "staging/api", TerraformVersion: "1.9.5", Detail: plan.FidelityResource,
		Counts: plan.Counts{Add: 1},
		Changes: []plan.ResourceChange{{
			Address: "aws_lambda_function.api", Action: plan.ActionCreate,
		}},
	}
	noop := plan.Unit{
		Name: "staging/idle", TerraformVersion: "1.9.5", Detail: plan.FidelityResource,
		Counts: plan.Counts{NoOp: 2},
		Changes: []plan.ResourceChange{
			{Address: "aws_s3_bucket.assets", Action: plan.ActionNoOp},
			{Address: "data.aws_iam_policy.read", Action: plan.ActionRead},
		},
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
	// FoldNoop is deliberately false (default) to exercise the regression path.
	s := config.Settings{}
	s.Render.FoldNoop = false

	got := Render(noopUnitReport(), s)
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

func TestRenderGoldenFoldNoop(t *testing.T) {
	s := config.Settings{}
	s.Render.FoldNoop = true

	got := Render(foldNoopReport(), s)
	golden := filepath.Join("testdata", "fold-noop.golden")
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
}

func TestMarker(t *testing.T) {
	if got := Marker("infra"); got != "<!-- gruntcmt:scope=infra -->" {
		t.Fatalf("Marker(infra) = %q", got)
	}
	if got := Marker(""); got != "<!-- gruntcmt:scope= -->" {
		t.Fatalf("Marker() = %q", got)
	}
}
