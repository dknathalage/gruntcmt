package ruleset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	owner, repo, path, ref, err := parseRef("org/shared//rules/base.yaml@v1")
	if err != nil || owner != "org" || repo != "shared" || path != "rules/base.yaml" || ref != "v1" {
		t.Fatalf("got %q %q %q %q err=%v", owner, repo, path, ref, err)
	}
	if _, _, _, _, err := parseRef("no-slash-slash"); err == nil {
		t.Fatal("expected error for missing //")
	}
}

func TestFetchRawContents(t *testing.T) {
	var gotPath, gotAccept, gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		gotRef = r.URL.Query().Get("ref")
		w.Write([]byte("rules:\n  - path: \"**\"\n"))
	}))
	defer srv.Close()

	f := &Fetcher{HTTP: http.DefaultClient, APIURL: strings.TrimRight(srv.URL, "/"), Token: "tok"}
	data, err := f.Fetch(context.Background(), "org/shared//rules/base.yaml@v1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "rules:") {
		t.Errorf("body = %q", data)
	}
	if gotPath != "/repos/org/shared/contents/rules/base.yaml" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAccept != "application/vnd.github.raw" {
		t.Errorf("accept = %q", gotAccept)
	}
	if gotRef != "v1" {
		t.Errorf("ref = %q", gotRef)
	}
}
