package analyze

import (
	"testing"

	"github.com/dknathalage/gruntcmt/internal/config"
	"github.com/dknathalage/gruntcmt/internal/plan"
)

func unit(name string, acts ...plan.Action) plan.Unit {
	u := plan.Unit{Name: name}
	for _, a := range acts {
		u.Changes = append(u.Changes, plan.ResourceChange{Action: a})
	}
	return u
}

func TestGroupByDepth(t *testing.T) {
	units := []plan.Unit{
		unit("production/database/primary", plan.ActionDelete),
		unit("production/networking", plan.ActionCreate),
		unit("staging/db1", plan.ActionNoOp),
	}
	r := Analyze(units, nil, config.Settings{GroupBy: 1, Detail: plan.FidelityResource})
	if len(r.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(r.Groups))
	}
	// production (has destroy) sorts before staging (no-op)
	if r.Groups[0].Key != "production" || r.Groups[1].Key != "staging" {
		t.Fatalf("group order = %q,%q", r.Groups[0].Key, r.Groups[1].Key)
	}
}

func TestGroupByTwoAndSingletonShortPath(t *testing.T) {
	units := []plan.Unit{
		unit("production/database/primary", plan.ActionUpdate),
		unit("production/networking", plan.ActionCreate), // only 2 segments
	}
	r := Analyze(units, nil, config.Settings{GroupBy: 2, Detail: plan.FidelityResource})
	keys := map[string]bool{}
	for _, g := range r.Groups {
		keys[g.Key] = true
	}
	if !keys["production/database"] || !keys["production/networking"] {
		t.Fatalf("keys = %v", keys)
	}
}

func TestPerUnitDetailResolved(t *testing.T) {
	s := config.Settings{GroupBy: 1, Detail: plan.FidelityResource,
		Overrides: []config.Override{{Path: "**/database/**", Detail: "attribute"}}}
	r := Analyze([]plan.Unit{unit("production/database/primary", plan.ActionUpdate)}, nil, s)
	if r.Groups[0].Units[0].Detail != plan.FidelityAttribute {
		t.Errorf("detail = %v, want attribute", r.Groups[0].Units[0].Detail)
	}
}

func TestTotalsAndGroupCountsAccumulate(t *testing.T) {
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
	r := Analyze(units, nil, config.Settings{GroupBy: 1, Detail: plan.FidelityResource})

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
	units := []plan.Unit{
		unit("a/x", plan.ActionCreate),
		unit("b/y", plan.ActionDelete),
	}
	r := Analyze(units, nil, config.Settings{GroupBy: 0, Detail: plan.FidelityResource})

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
	units := []plan.Unit{
		unit("a/destroy", plan.ActionDelete),
		{
			Name:    "a/noop",
			Changes: []plan.ResourceChange{}, // No changes at all
		},
	}
	r := Analyze(units, nil, config.Settings{GroupBy: 1, Detail: plan.FidelityResource})

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
