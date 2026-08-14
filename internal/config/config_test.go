package config

import (
	"os"
	"path/filepath"
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

func TestMergeBoolToggleIsAdditive(t *testing.T) {
	// Demonstrates that HideUnchanged is additive: once set true in any layer, it stays true.
	layerA := File{Render: Render{HideUnchanged: true}}
	layerB := File{Detail: "resource"}
	m := Merge(layerA, layerB)
	if !m.Render.HideUnchanged {
		t.Errorf("merged.Render.HideUnchanged = %v, want true (additive)", m.Render.HideUnchanged)
	}
}

func TestDiscoverWalksUpAndStopsAtGit(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure
	repoDir := filepath.Join(tmpDir, "repo")
	envDir := filepath.Join(repoDir, "env")
	unitDir := filepath.Join(envDir, "unit")
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Put .gruntcmt.yaml at repo root and .git directory at repo root
	configPath := filepath.Join(repoDir, ".gruntcmt.yaml")
	if err := os.WriteFile(configPath, []byte("detail: resource\n"), 0644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	gitDir := filepath.Join(repoDir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatalf("Mkdir .git: %v", err)
	}

	// Discover from nested dir should find config at repo root
	path, found := Discover(unitDir)
	if !found || path != configPath {
		t.Errorf("Discover(%q) = %q, %v; want %q, true", unitDir, path, found, configPath)
	}

	// Test stop-at-git behavior: create a separate git repo with no config to verify it stops at .git
	gitOnlyDir := filepath.Join(tmpDir, "gitonly")
	gitOnlyDeepDir := filepath.Join(gitOnlyDir, "subdir")
	if err := os.MkdirAll(gitOnlyDeepDir, 0755); err != nil {
		t.Fatalf("MkdirAll gitonly: %v", err)
	}
	// Create only .git, no config
	gitOnlyGitDir := filepath.Join(gitOnlyDir, ".git")
	if err := os.Mkdir(gitOnlyGitDir, 0755); err != nil {
		t.Fatalf("Mkdir gitonly/.git: %v", err)
	}

	// From a dir under .git with no config anywhere, Discover should stop at .git and return found=false
	path, found = Discover(gitOnlyDeepDir)
	if found {
		t.Errorf("Discover(%q) = %q, %v; want empty, false (stops at .git)", gitOnlyDeepDir, path, found)
	}
}

func TestParseFidelityInvalid(t *testing.T) {
	// Test that invalid value returns error
	_, err := ParseFidelity("bogus")
	if err == nil {
		t.Error("ParseFidelity(\"bogus\") should return error, got nil")
	}

	// Spot-check the three valid values
	tests := []struct {
		input string
		want  plan.Fidelity
	}{
		{"summary", plan.FidelitySummary},
		{"resource", plan.FidelityResource},
		{"attribute", plan.FidelityAttribute},
	}
	for _, tt := range tests {
		got, err := ParseFidelity(tt.input)
		if err != nil {
			t.Errorf("ParseFidelity(%q) returned error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseFidelity(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
