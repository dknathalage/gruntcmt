// Package ghctx resolves the repository, pull request, commit, token, and
// comment scope for gruntcmt. Every value prefers a CI environment variable and
// falls back to a local git/gh invocation, so behavior is identical locally and
// in CI. Only the --out gh path calls into this package.
package ghctx

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Token resolves a GitHub token: $GITHUB_TOKEN, then $GH_TOKEN, then `gh auth token`.
func Token() string {
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

// Repo resolves "owner/name" from $GITHUB_REPOSITORY, else the local git origin.
func Repo() (string, error) {
	if v := os.Getenv("GITHUB_REPOSITORY"); v != "" {
		return v, nil
	}
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	if r, ok := parseRepoRemote(strings.TrimSpace(string(out))); ok {
		return r, nil
	}
	return "", fmt.Errorf("cannot parse owner/name from git origin %q", strings.TrimSpace(string(out)))
}

var repoRe = regexp.MustCompile(`[:/]([^/:]+)/([^/]+?)(?:\.git)?$`)

func parseRepoRemote(url string) (string, bool) {
	m := repoRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

var pullRefRe = regexp.MustCompile(`^refs/pull/(\d+)/`)

func prFromRef(ref string) (int, bool) {
	if m := pullRefRe.FindStringSubmatch(ref); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, true
		}
	}
	return 0, false
}

// PR resolves the pull-request number: $GITHUB_REF, then the event payload at
// $GITHUB_EVENT_PATH, then `gh pr view` for the current branch.
func PR() (int, error) {
	if n, ok := prFromRef(os.Getenv("GITHUB_REF")); ok {
		return n, nil
	}
	if p := os.Getenv("GITHUB_EVENT_PATH"); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			var ev struct {
				Number      int `json:"number"`
				PullRequest struct {
					Number int `json:"number"`
				} `json:"pull_request"`
			}
			if json.Unmarshal(raw, &ev) == nil {
				if ev.PullRequest.Number != 0 {
					return ev.PullRequest.Number, nil
				}
				if ev.Number != 0 {
					return ev.Number, nil
				}
			}
		}
	}
	out, err := exec.Command("gh", "pr", "view", "--json", "number", "-q", ".number").Output()
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Commit resolves the commit SHA: $GITHUB_SHA, else `git rev-parse HEAD`.
func Commit() string {
	if v := os.Getenv("GITHUB_SHA"); v != "" {
		return v
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Scope derives the comment scope from the first path argument's basename.
func Scope(paths []string) string {
	if len(paths) == 0 {
		return "plan"
	}
	return scopeFromPath(paths[0])
}

func scopeFromPath(p string) string {
	base := filepath.Base(filepath.Clean(p))
	if base == "." || base == "/" || base == "" {
		return "plan"
	}
	return base
}
