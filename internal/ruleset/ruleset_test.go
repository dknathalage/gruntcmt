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

func TestParseRejectsDedicatedWithoutScope(t *testing.T) {
	yaml := "rules:\n  - path: \"**\"\n    dedicated-comment: true\n"
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for dedicated-comment without scope, got nil")
	}
}

func TestDedicatedFallbackToNonDedicated(t *testing.T) {
	// Ruleset with non-dedicated rule setting title and group-by,
	// and a dedicated rule that sets neither
	const sampleFallback = `
rules:
  - path: "**"
    title: "main title"
    group-by: 2
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    delete: attribute
`
	rs, err := Parse([]byte(sampleFallback))
	if err != nil {
		t.Fatal(err)
	}

	// When the dedicated scope rule doesn't set title/group-by,
	// should fall back to the non-dedicated rule
	if got := rs.Title("security", true); got != "main title" {
		t.Errorf("dedicated title should fall back to non-dedicated = %q, want 'main title'", got)
	}
	if got := rs.GroupBy("security", true); got != 2 {
		t.Errorf("dedicated group-by should fall back to non-dedicated = %d, want 2", got)
	}

	// Verify that a dedicated rule that DOES set its own value still wins
	const sampleDedicatedWins = `
rules:
  - path: "**"
    title: "main title"
    group-by: 2
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    title: "security title"
    group-by: 5
    delete: attribute
`
	rs2, err := Parse([]byte(sampleDedicatedWins))
	if err != nil {
		t.Fatal(err)
	}
	if got := rs2.Title("security", true); got != "security title" {
		t.Errorf("dedicated title should win = %q, want 'security title'", got)
	}
	if got := rs2.GroupBy("security", true); got != 5 {
		t.Errorf("dedicated group-by should win = %d, want 5", got)
	}
}
