package config

import (
	"testing"

	"github.com/dknathalage/gruntcmt/internal/plan"
)

func gb(n int) *int { return &n }

func TestMergeLastWins(t *testing.T) {
	global := File{GroupBy: gb(1), Detail: "summary"}
	repo := File{Detail: "resource"}
	m := Merge(global, repo)
	if *m.GroupBy != 1 || m.Detail != "resource" {
		t.Fatalf("merged = %+v", m)
	}
}

func TestDetailForOverrideLastMatchWins(t *testing.T) {
	s := Settings{
		Detail: plan.FidelityResource,
		Overrides: []Override{
			{Path: "**/database/**", Detail: "attribute"},
			{Path: "development/**", Detail: "summary"},
		},
	}
	if got := s.DetailFor("production/database/primary"); got != plan.FidelityAttribute {
		t.Errorf("prod/db = %v, want attribute", got)
	}
	if got := s.DetailFor("development/database/db1"); got != plan.FidelitySummary {
		t.Errorf("dev/db = %v (last match wins) want summary", got)
	}
	if got := s.DetailFor("staging/eks"); got != plan.FidelityResource {
		t.Errorf("no match = %v, want default resource", got)
	}
}

func TestDetailForFlagHammerBeatsOverrides(t *testing.T) {
	s := Settings{Detail: plan.FidelitySummary, DetailSet: true,
		Overrides: []Override{{Path: "**", Detail: "attribute"}}}
	if got := s.DetailFor("anything"); got != plan.FidelitySummary {
		t.Errorf("flag hammer = %v, want summary", got)
	}
}
