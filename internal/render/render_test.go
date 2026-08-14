package render

import (
	"flag"
	"os"
	"path/filepath"
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
