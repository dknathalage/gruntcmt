// Package gh is gruntcmt's only networked component: a minimal GitHub REST client
// that creates or updates a pull-request comment in place. It is used solely by
// the --out gh path; the rest of gruntcmt never touches the network.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Client posts issue/PR comments via the GitHub REST API.
type Client struct {
	HTTP   *http.Client
	APIURL string // e.g. https://api.github.com (no trailing slash)
	Token  string
}

type comment struct {
	ID      int64  `json:"id"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

// UpsertComment finds the issue comment on repo#pr whose body contains marker and
// PATCHes it with body; if none matches it POSTs a new comment. Returns the
// comment's html_url.
func (c *Client) UpsertComment(ctx context.Context, repo string, pr int, marker, body string) (string, error) {
	id, err := c.findByMarker(ctx, repo, pr, marker)
	if err != nil {
		return "", err
	}
	payload := map[string]string{"body": body}
	if id != 0 {
		var out comment
		url := fmt.Sprintf("%s/repos/%s/issues/comments/%d", c.APIURL, repo, id)
		if err := c.send(ctx, http.MethodPatch, url, payload, &out); err != nil {
			return "", err
		}
		return out.HTMLURL, nil
	}
	var out comment
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments", c.APIURL, repo, pr)
	if err := c.send(ctx, http.MethodPost, url, payload, &out); err != nil {
		return "", err
	}
	return out.HTMLURL, nil
}

func (c *Client) findByMarker(ctx context.Context, repo string, pr int, marker string) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/issues/%d/comments?per_page=100", c.APIURL, repo, pr)
	for url != "" {
		var page []comment
		next, err := c.get(ctx, url, &page)
		if err != nil {
			return 0, err
		}
		for _, cm := range page {
			if strings.Contains(cm.Body, marker) {
				return cm.ID, nil
			}
		}
		url = next
	}
	return 0, nil
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>\s*;\s*rel="next"`)

// get performs a GET, decodes the JSON body into out, and returns the next-page
// URL parsed from the Link header ("" when there is no next page).
func (c *Client) get(ctx context.Context, url string, out any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", apiError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return "", err
	}
	next := ""
	if m := nextLinkRe.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
		next = m[1]
	}
	return next, nil
}

func (c *Client) send(ctx context.Context, method, url string, payload, out any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError(resp)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
}

func apiError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("github api %s: %s", resp.Status, strings.TrimSpace(string(b)))
}
