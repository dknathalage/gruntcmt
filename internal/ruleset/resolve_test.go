package ruleset

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dknathalage/gruntcmt/internal/plan"
)

func TestResolveMergesBaseThenLocal(t *testing.T) {
	// base defines create=summary; local overrides create=attribute (local wins)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("rules:\n  - path: \"**\"\n    create: summary\n    delete: resource\n"))
	}))
	defer srv.Close()

	local, _ := Parse([]byte("base: org/shared//base.yaml@v1\nrules:\n  - path: \"**\"\n    create: attribute\n"))
	f := &Fetcher{HTTP: http.DefaultClient, APIURL: strings.TrimRight(srv.URL, "/")}
	merged, err := Resolve(context.Background(), local, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Rules) != 2 {
		t.Fatalf("merged rules = %d, want 2 (base + local)", len(merged.Rules))
	}
	// base delete=resource still applies; local create=attribute wins over base summary
	if got := merged.Detail("x", plan.ActionCreate); got != plan.FidelityAttribute {
		t.Errorf("create = %v, want %v (local wins)", got, plan.FidelityAttribute)
	}
	if got := merged.Detail("x", plan.ActionDelete); got != plan.FidelityResource {
		t.Errorf("delete = %v, want %v (from base)", got, plan.FidelityResource)
	}
}

func TestResolveDetectsCycle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// base points back at itself
		fmt.Fprintf(w, "base: org/shared//loop.yaml@v1\nrules: []\n")
	}))
	defer srv.Close()

	local, _ := Parse([]byte("base: org/shared//loop.yaml@v1\nrules: []\n"))
	f := &Fetcher{HTTP: http.DefaultClient, APIURL: strings.TrimRight(srv.URL, "/")}
	if _, err := Resolve(context.Background(), local, f); err == nil {
		t.Fatal("expected cycle error")
	}
}
