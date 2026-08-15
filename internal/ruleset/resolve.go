package ruleset

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxBaseDepth = 10

func Resolve(ctx context.Context, rs Ruleset, f *Fetcher) (Ruleset, error) {
	return resolve(ctx, rs, f, map[string]bool{}, 0)
}

func resolve(ctx context.Context, rs Ruleset, f *Fetcher, seen map[string]bool, depth int) (Ruleset, error) {
	if rs.Base == "" {
		return Ruleset{Rules: append([]Rule(nil), rs.Rules...)}, nil
	}
	if depth >= maxBaseDepth {
		return Ruleset{}, fmt.Errorf("base chain too deep (>%d)", maxBaseDepth)
	}
	if seen[rs.Base] {
		return Ruleset{}, fmt.Errorf("base cycle detected at %q", rs.Base)
	}
	seen[rs.Base] = true
	data, err := f.Fetch(ctx, rs.Base)
	if err != nil {
		return Ruleset{}, err
	}
	baseRS, err := Parse(data)
	if err != nil {
		return Ruleset{}, fmt.Errorf("base %q: %w", rs.Base, err)
	}
	resolvedBase, err := resolve(ctx, baseRS, f, seen, depth+1)
	if err != nil {
		return Ruleset{}, err
	}
	return Ruleset{Rules: append(resolvedBase.Rules, rs.Rules...)}, nil
}

func DefaultToken() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
