package ruleset

import (
	"testing"

	"github.com/dknathalage/gruntcmt/internal/plan"
)

const sample = `
rules:
  - path: "**"
    title: "gruntcmt scenarios"
    group-by: 1
    create: summary
    update: resource
    delete: attribute
    replace: attribute
    noop: summary
  - path: "**/database*"
    create: attribute
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    title: "Security plan"
    delete: attribute
`

func mustParse(t *testing.T) Ruleset {
	t.Helper()
	rs, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestDetailPerAction(t *testing.T) {
	rs := mustParse(t)
	if got := rs.Detail("prod/networking", plan.ActionCreate); got != plan.FidelitySummary {
		t.Errorf("create default = %v, want summary", got)
	}
	if got := rs.Detail("prod/networking", plan.ActionDelete); got != plan.FidelityAttribute {
		t.Errorf("delete default = %v, want attribute", got)
	}
	// database override raises create to attribute (last match wins)
	if got := rs.Detail("prod/database-primary", plan.ActionCreate); got != plan.FidelityAttribute {
		t.Errorf("db create = %v, want attribute", got)
	}
	// unspecified action falls back to built-in default (resource)
	rsMin, _ := Parse([]byte("rules:\n  - path: \"**\"\n    delete: attribute\n"))
	if got := rsMin.Detail("x", plan.ActionUpdate); got != plan.FidelityResource {
		t.Errorf("update fallback = %v, want resource", got)
	}
}

func TestAssignAndMeta(t *testing.T) {
	rs := mustParse(t)
	if scope, ded := rs.Assign("prod/security/iam"); !ded || scope != "security" {
		t.Errorf("security assign = %q,%v", scope, ded)
	}
	if _, ded := rs.Assign("prod/networking"); ded {
		t.Error("networking should be main (not dedicated)")
	}
	if got := rs.DedicatedScopes(); len(got) != 1 || got[0] != "security" {
		t.Errorf("dedicated scopes = %v", got)
	}
	if got := rs.Title("", false); got != "gruntcmt scenarios" {
		t.Errorf("main title = %q", got)
	}
	if got := rs.Title("security", true); got != "Security plan" {
		t.Errorf("dedicated title = %q", got)
	}
	if got := rs.GroupBy("", false); got != 1 {
		t.Errorf("main group-by = %d", got)
	}
}

func TestParseRejectsBadDetail(t *testing.T) {
	if _, err := Parse([]byte("rules:\n  - path: \"**\"\n    create: bogus\n")); err == nil {
		t.Fatal("expected error for invalid detail")
	}
}
