package ruleset

import (
	"fmt"
	"os"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/dknathalage/gruntcmt/internal/plan"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	Path             string `yaml:"path"`
	Title            string `yaml:"title"`
	GroupBy          *int   `yaml:"group-by"`
	DedicatedComment bool   `yaml:"dedicated-comment"`
	Scope            string `yaml:"scope"`
	Create           string `yaml:"create"`
	Update           string `yaml:"update"`
	Delete           string `yaml:"delete"`
	Replace          string `yaml:"replace"`
	Noop             string `yaml:"noop"`
}

type Ruleset struct {
	Rules []Rule `yaml:"rules"`
}

// Default is the built-in ruleset used when no config file is present:
// a single flat group with full attribute detail for every real change.
func Default() Ruleset {
	zero := 0
	return Ruleset{Rules: []Rule{{
		Path:    "**",
		Title:   "Terragrunt plan",
		GroupBy: &zero,
		Create:  "attribute",
		Update:  "attribute",
		Delete:  "attribute",
		Replace: "attribute",
		Noop:    "summary",
	}}}
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

func Parse(data []byte) (Ruleset, error) {
	var rs Ruleset
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return rs, err
	}
	for i, r := range rs.Rules {
		if r.DedicatedComment && r.Scope == "" {
			return rs, fmt.Errorf("rule %d (%s): dedicated-comment requires a non-empty scope", i, r.Path)
		}
		for _, v := range []string{r.Create, r.Update, r.Delete, r.Replace, r.Noop} {
			if v != "" {
				if _, err := ParseFidelity(v); err != nil {
					return rs, fmt.Errorf("rule %d (%s): %w", i, r.Path, err)
				}
			}
		}
	}
	return rs, nil
}

func Load(path string) (Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Ruleset{}, err
	}
	rs, err := Parse(data)
	if err != nil {
		return rs, fmt.Errorf("%s: %w", path, err)
	}
	return rs, nil
}

func actionField(r Rule, a plan.Action) string {
	switch a {
	case plan.ActionCreate:
		return r.Create
	case plan.ActionUpdate:
		return r.Update
	case plan.ActionDelete:
		return r.Delete
	case plan.ActionReplace:
		return r.Replace
	default: // NoOp, Read
		return r.Noop
	}
}

func defaultDetail(a plan.Action) plan.Fidelity {
	if a == plan.ActionNoOp || a == plan.ActionRead {
		return plan.FidelitySummary
	}
	return plan.FidelityResource
}

func match(pattern, path string) bool {
	ok, _ := doublestar.Match(pattern, path)
	return ok
}

func (rs Ruleset) Detail(unitPath string, a plan.Action) plan.Fidelity {
	detail := defaultDetail(a)
	for _, r := range rs.Rules {
		if !match(r.Path, unitPath) {
			continue
		}
		if v := actionField(r, a); v != "" {
			if f, err := ParseFidelity(v); err == nil {
				detail = f
			}
		}
	}
	return detail
}

func (rs Ruleset) Assign(unitPath string) (string, bool) {
	scope, dedicated := "", false
	for _, r := range rs.Rules {
		if r.DedicatedComment && match(r.Path, unitPath) {
			scope, dedicated = r.Scope, true
		}
	}
	return scope, dedicated
}

func (rs Ruleset) DedicatedScopes() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rs.Rules {
		if r.DedicatedComment && r.Scope != "" && !seen[r.Scope] {
			seen[r.Scope] = true
			out = append(out, r.Scope)
		}
	}
	return out
}

func (rs Ruleset) Title(scope string, dedicated bool) string {
	if !dedicated {
		// Non-dedicated: last non-dedicated rule with title, else default
		title := "Terragrunt plan"
		for _, r := range rs.Rules {
			if r.Title != "" && !r.DedicatedComment {
				title = r.Title
			}
		}
		return title
	}

	// Dedicated: try dedicated rule with matching scope first
	dedicatedTitle := ""
	nonDedicatedTitle := ""
	for _, r := range rs.Rules {
		if r.Title == "" {
			continue
		}
		if r.DedicatedComment && r.Scope == scope {
			dedicatedTitle = r.Title
		} else if !r.DedicatedComment {
			nonDedicatedTitle = r.Title
		}
	}

	if dedicatedTitle != "" {
		return dedicatedTitle
	}
	if nonDedicatedTitle != "" {
		return nonDedicatedTitle
	}
	return "Terragrunt plan"
}

func (rs Ruleset) GroupBy(scope string, dedicated bool) int {
	if !dedicated {
		// Non-dedicated: last non-dedicated rule with group-by, else default
		gb := 1
		for _, r := range rs.Rules {
			if r.GroupBy != nil && !r.DedicatedComment {
				gb = *r.GroupBy
			}
		}
		return gb
	}

	// Dedicated: try dedicated rule with matching scope first
	var dedicatedGB *int
	var nonDedicatedGB *int
	for _, r := range rs.Rules {
		if r.GroupBy == nil {
			continue
		}
		if r.DedicatedComment && r.Scope == scope {
			dedicatedGB = r.GroupBy
		} else if !r.DedicatedComment {
			nonDedicatedGB = r.GroupBy
		}
	}

	if dedicatedGB != nil {
		return *dedicatedGB
	}
	if nonDedicatedGB != nil {
		return *nonDedicatedGB
	}
	return 1
}
