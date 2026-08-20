package ghctx

import "testing"

func TestParseRepoRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:dknathalage/gruntcmt.git":       "dknathalage/gruntcmt",
		"https://github.com/dknathalage/gruntcmt.git":   "dknathalage/gruntcmt",
		"https://github.com/dknathalage/gruntcmt":       "dknathalage/gruntcmt",
		"ssh://git@github.com/dknathalage/gruntcmt.git": "dknathalage/gruntcmt",
	}
	for in, want := range cases {
		got, ok := parseRepoRemote(in)
		if !ok || got != want {
			t.Errorf("parseRepoRemote(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := parseRepoRemote("not-a-remote"); ok {
		t.Error("expected failure for junk remote")
	}
}

func TestPRFromRef(t *testing.T) {
	if n, ok := prFromRef("refs/pull/42/merge"); !ok || n != 42 {
		t.Errorf("got %d,%v want 42", n, ok)
	}
	if _, ok := prFromRef("refs/heads/main"); ok {
		t.Error("expected no PR for branch ref")
	}
}

func TestScopeFromPath(t *testing.T) {
	cases := map[string]string{
		"envs/prod":  "prod",
		"envs/prod/": "prod",
		"out":        "out",
		".":          "plan",
		"":           "plan",
		"/":          "plan",
	}
	for in, want := range cases {
		if got := scopeFromPath(in); got != want {
			t.Errorf("scopeFromPath(%q) = %q want %q", in, got, want)
		}
	}
}

func TestScopeUsesFirstPath(t *testing.T) {
	if got := Scope([]string{"envs/staging", "envs/prod"}); got != "staging" {
		t.Errorf("Scope = %q want staging", got)
	}
	if got := Scope(nil); got != "plan" {
		t.Errorf("Scope(nil) = %q want plan", got)
	}
}
