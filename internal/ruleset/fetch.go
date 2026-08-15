package ruleset

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Fetcher struct {
	HTTP   *http.Client
	APIURL string
	Token  string
}

func parseRef(ref string) (owner, repo, path, gitref string, err error) {
	i := strings.Index(ref, "//")
	if i < 0 {
		return "", "", "", "", fmt.Errorf("invalid base ref %q (want owner/repo//path[@ref])", ref)
	}
	repoPart, rest := ref[:i], ref[i+2:]
	rp := strings.SplitN(repoPart, "/", 2)
	if len(rp) != 2 || rp[0] == "" || rp[1] == "" {
		return "", "", "", "", fmt.Errorf("invalid base ref %q (want owner/repo//path)", ref)
	}
	owner, repo = rp[0], rp[1]
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		path, gitref = rest[:at], rest[at+1:]
	} else {
		path = rest
	}
	if path == "" {
		return "", "", "", "", fmt.Errorf("invalid base ref %q (empty path)", ref)
	}
	return owner, repo, path, gitref, nil
}

func (f *Fetcher) Fetch(ctx context.Context, ref string) ([]byte, error) {
	owner, repo, path, gitref, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", f.APIURL, owner, repo, path)
	if gitref != "" {
		u += "?ref=" + url.QueryEscape(gitref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if f.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Token)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetch base %q: %s: %s", ref, resp.Status, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}
