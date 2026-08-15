package analyze

import (
	"testing"

	"github.com/dknathalage/gruntcmt/internal/plan"
	"github.com/dknathalage/gruntcmt/internal/ruleset"
)

func unit(name string, acts ...plan.Action) plan.Unit {
	u := plan.Unit{Name: name}
	for _, a := range acts {
		u.Changes = append(u.Changes, plan.ResourceChange{Action: a})
	}
	u.Counts = plan.Count(u.Changes)
	return u
}

// ---- New tests (Task 5) ----

func TestAnalyzeSplitsDedicatedComment(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 1
    delete: attribute
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    delete: attribute
`))
	units := []plan.Unit{
		unit("prod/networking", plan.ActionCreate),
		unit("prod/security/iam", plan.ActionDelete),
	}
	reports := Analyze(units, nil, rs, "infra")
	if len(reports) != 2 {
		t.Fatalf("reports = %d, want 2 (main + security)", len(reports))
	}
	if reports[0].Scope != "infra" {
		t.Errorf("main scope = %q", reports[0].Scope)
	}
	if reports[1].Scope != "security" {
		t.Errorf("dedicated scope = %q", reports[1].Scope)
	}
}

func TestAnalyzeStampsPerChangeDetail(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    create: summary
    delete: attribute
`))
	u := unit("prod/db", plan.ActionCreate, plan.ActionDelete)
	reports := Analyze([]plan.Unit{u}, nil, rs, "infra")
	got := reports[0].Groups[0].Units[0].Changes
	if got[0].Detail != plan.FidelitySummary || got[1].Detail != plan.FidelityAttribute {
		t.Fatalf("details = %v,%v", got[0].Detail, got[1].Detail)
	}
}

// ---- Updated pre-existing tests (old Analyze(config.Settings) → new signature) ----

func TestGroupByDepth(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 1
`))
	units := []plan.Unit{
		unit("production/database/primary", plan.ActionDelete),
		unit("production/networking", plan.ActionCreate),
		unit("staging/db1", plan.ActionNoOp),
	}
	reports := Analyze(units, nil, rs, "main")
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	r := reports[0]
	if len(r.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(r.Groups))
	}
	// production (has destroy) sorts before staging (no-op)
	if r.Groups[0].Key != "production" || r.Groups[1].Key != "staging" {
		t.Fatalf("group order = %q,%q", r.Groups[0].Key, r.Groups[1].Key)
	}
}

func TestGroupByTwoAndSingletonShortPath(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 2
`))
	units := []plan.Unit{
		unit("production/database/primary", plan.ActionUpdate),
		unit("production/networking", plan.ActionCreate), // only 2 segments
	}
	reports := Analyze(units, nil, rs, "main")
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	r := reports[0]
	keys := map[string]bool{}
	for _, g := range r.Groups {
		keys[g.Key] = true
	}
	if !keys["production/database"] || !keys["production/networking"] {
		t.Fatalf("keys = %v", keys)
	}
}

func TestPerUnitDetailResolved(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 1
    update: resource
  - path: "**/database/**"
    update: attribute
`))
	reports := Analyze([]plan.Unit{unit("production/database/primary", plan.ActionUpdate)}, nil, rs, "main")
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	if reports[0].Groups[0].Units[0].Changes[0].Detail != plan.FidelityAttribute {
		t.Errorf("detail = %v, want attribute", reports[0].Groups[0].Units[0].Changes[0].Detail)
	}
}

func TestTotalsAndGroupCountsAccumulate(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 1
`))
	units := []plan.Unit{
		{
			Name:    "a/x",
			Counts:  plan.Counts{Add: 2, Change: 1},
			Changes: []plan.ResourceChange{},
		},
		{
			Name:    "a/y",
			Counts:  plan.Counts{Destroy: 1, Replace: 1, Add: 1},
			Changes: []plan.ResourceChange{},
		},
	}
	reports := Analyze(units, nil, rs, "main")
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	r := reports[0]

	// Check that group counts accumulate
	if len(r.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(r.Groups))
	}
	group := r.Groups[0]
	if group.Counts.Add != 3 || group.Counts.Change != 1 || group.Counts.Destroy != 1 || group.Counts.Replace != 1 {
		t.Errorf("group counts = %+v, want Add:3 Change:1 Destroy:1 Replace:1", group.Counts)
	}

	// Check that totals accumulate
	if r.Totals.Add != 3 || r.Totals.Change != 1 || r.Totals.Destroy != 1 || r.Totals.Replace != 1 {
		t.Errorf("report totals = %+v, want Add:3 Change:1 Destroy:1 Replace:1", r.Totals)
	}
}

func TestGroupByZeroFlat(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 0
`))
	units := []plan.Unit{
		unit("a/x", plan.ActionCreate),
		unit("b/y", plan.ActionDelete),
	}
	reports := Analyze(units, nil, rs, "main")
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	r := reports[0]

	// GroupBy:0 should produce exactly one group with empty key
	if len(r.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(r.Groups))
	}
	if r.Groups[0].Key != "" {
		t.Errorf("group key = %q, want empty string", r.Groups[0].Key)
	}
	// Both units should be in that single group
	if len(r.Groups[0].Units) != 2 {
		t.Errorf("expected 2 units in flat group, got %d", len(r.Groups[0].Units))
	}
}

func TestZeroChangeUnitSortsLast(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 1
`))
	units := []plan.Unit{
		unit("a/destroy", plan.ActionDelete),
		{
			Name:    "a/noop",
			Changes: []plan.ResourceChange{}, // No changes at all
		},
	}
	reports := Analyze(units, nil, rs, "main")
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	r := reports[0]

	if len(r.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(r.Groups))
	}
	group := r.Groups[0]
	if len(group.Units) != 2 {
		t.Fatalf("expected 2 units in group, got %d", len(group.Units))
	}

	// Destructive unit (severity 5) should come first
	if group.Units[0].Name != "a/destroy" {
		t.Errorf("first unit = %q, want a/destroy", group.Units[0].Name)
	}
	// Zero-change unit (severity 0) should come last
	if group.Units[1].Name != "a/noop" {
		t.Errorf("second unit = %q, want a/noop", group.Units[1].Name)
	}
}
