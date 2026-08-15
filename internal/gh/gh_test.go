package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(url string) *Client {
	return &Client{HTTP: http.DefaultClient, APIURL: strings.TrimRight(url, "/"), Token: "t0ken"}
}

func TestUpsertCreatesWhenNoMatch(t *testing.T) {
	var posted, auth, apiVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		apiVersion = r.Header.Get("X-GitHub-Api-Version")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues/7/comments"):
			w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/issues/7/comments"):
			var b struct {
				Body string `json:"body"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			posted = b.Body
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":11,"html_url":"https://gh/comment/11"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	url, err := newTestClient(srv.URL).UpsertComment(context.Background(), "o/r", 7, "MARK", "hello MARK")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://gh/comment/11" {
		t.Errorf("url = %q, want the created comment url", url)
	}
	if posted != "hello MARK" {
		t.Errorf("posted body = %q", posted)
	}
	if auth != "Bearer t0ken" {
		t.Errorf("Authorization = %q", auth)
	}
	if apiVersion != "2022-11-28" {
		t.Errorf("X-GitHub-Api-Version = %q", apiVersion)
	}
}

func TestUpsertUpdatesWhenMarkerFound(t *testing.T) {
	var patchedID, patchedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`[{"id":5,"body":"unrelated"},{"id":6,"body":"contains MARK here"}]`))
		case http.MethodPatch:
			patchedID = strings.TrimPrefix(r.URL.Path, "/repos/o/r/issues/comments/")
			var b struct {
				Body string `json:"body"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			patchedBody = b.Body
			w.Write([]byte(`{"id":6,"html_url":"https://gh/comment/6"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	url, err := newTestClient(srv.URL).UpsertComment(context.Background(), "o/r", 9, "MARK", "new MARK body")
	if err != nil {
		t.Fatal(err)
	}
	if patchedID != "6" {
		t.Errorf("patched id = %q, want 6", patchedID)
	}
	if patchedBody != "new MARK body" {
		t.Errorf("patched body = %q", patchedBody)
	}
	if url != "https://gh/comment/6" {
		t.Errorf("url = %q", url)
	}
}

func TestUpsertFollowsPagination(t *testing.T) {
	var patched bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patched = true
			w.Write([]byte(`{"id":6,"html_url":"https://gh/c/6"}`))
			return
		}
		if r.URL.Query().Get("page") == "2" {
			w.Write([]byte(`[{"id":6,"body":"page two MARK"}]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<http://%s/repos/o/r/issues/8/comments?page=2>; rel="next"`, r.Host))
		w.Write([]byte(`[{"id":5,"body":"page one no match"}]`))
	}))
	defer srv.Close()

	url, err := newTestClient(srv.URL).UpsertComment(context.Background(), "o/r", 8, "MARK", "body")
	if err != nil {
		t.Fatal(err)
	}
	if !patched || url != "https://gh/c/6" {
		t.Fatalf("patched=%v url=%q (expected to find match on page 2 and patch it)", patched, url)
	}
}

func TestUpsertErrorsOnAPIFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).UpsertComment(context.Background(), "o/r", 1, "MARK", "body"); err == nil {
		t.Fatal("expected error on 401 response")
	}
}
