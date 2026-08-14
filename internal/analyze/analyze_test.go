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
