package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/dknathalage/gruntcmt/internal/plan"
	"gopkg.in/yaml.v3"
)

type Render struct {
	Title         string            `yaml:"title"`
	Emoji         map[string]string `yaml:"emoji"`
	HideUnchanged bool              `yaml:"hide-unchanged"`
	FoldNoop      bool              `yaml:"fold-noop"`
}

type Override struct {
	Path   string `yaml:"path"`
	Detail string `yaml:"detail"`
}

type File struct {
	GroupBy   *int       `yaml:"group-by"`
	Detail    string     `yaml:"detail"`
	Input     string     `yaml:"input"`
	Render    Render     `yaml:"render"`
	Overrides []Override `yaml:"overrides"`
}

type Settings struct {
	Scope     string
	Name      string
	Commit    string
	GroupBy   int
	Input     string
	Detail    plan.Fidelity
	DetailSet bool
	Render    Render
	Overrides []Override
}

func ParseFidelity(s string) (plan.Fidelity, error) {
	switch s {
	case "summary":
		return plan.FidelitySummary, nil
	case "resource":
		return plan.FidelityResource, nil
	case "attribute":
		return plan.FidelityAttribute, nil
	default:
		return 0, fmt.Errorf("invalid detail %q (want summary|resource|attribute)", s)
	}
}

func LoadFile(path string) (File, error) {
	var f File
	raw, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

func Discover(startDir string) (string, bool) {
	dir := startDir
	for {
		p := filepath.Join(dir, ".gruntcmt.yaml")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return "", false // stop at repo root
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func Merge(layers ...File) File {
	var out File
	for _, l := range layers {
		if l.GroupBy != nil {
			out.GroupBy = l.GroupBy
		}
		if l.Detail != "" {
			out.Detail = l.Detail
		}
		if l.Input != "" {
			out.Input = l.Input
		}
		if l.Render.Title != "" {
			out.Render.Title = l.Render.Title
		}
		if l.Render.Emoji != nil {
			out.Render.Emoji = l.Render.Emoji
		}
		if l.Render.HideUnchanged {
			out.Render.HideUnchanged = true
		}
		if l.Render.FoldNoop {
			out.Render.FoldNoop = true
		}
		if l.Overrides != nil {
			out.Overrides = l.Overrides
		}
	}
	return out
}

func (s Settings) DetailFor(unitPath string) plan.Fidelity {
	if s.DetailSet {
		return s.Detail
	}
	detail := s.Detail
	for _, o := range s.Overrides {
		if ok, _ := doublestar.Match(o.Path, unitPath); ok {
			if f, err := ParseFidelity(o.Detail); err == nil {
				detail = f
			}
		}
	}
	return detail
}
