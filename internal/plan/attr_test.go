package plan

import (
	"encoding/json"
	"testing"
)

func find(attrs []AttributeChange, path string) (AttributeChange, bool) {
	for _, a := range attrs {
		if a.Path == path {
			return a, true
		}
	}
	return AttributeChange{}, false
}

func TestAttributeDiff(t *testing.T) {
	u := load(t, "attr.json")
	attrs := u.Changes[0].Attributes

	ev, ok := find(attrs, "engine_version")
	if !ok || ev.Before != "14.7" || ev.After != "15.4" || !ev.ForcesNew {
		t.Errorf("engine_version = %+v", ev)
	}
	as, _ := find(attrs, "allocated_storage")
	if as.Before != "100" || as.After != "200" {
		t.Errorf("allocated_storage = %+v", as)
	}
	pw, ok := find(attrs, "password")
	if !ok || !pw.Sensitive {
		t.Errorf("password = %+v, want sensitive add", pw)
	}
	if _, ok := find(attrs, "name"); ok {
		t.Error("unchanged 'name' should not appear")
	}
	if u.Changes[0].Unchanged != 1 { // only "name" unchanged
		t.Errorf("unchanged = %d, want 1", u.Changes[0].Unchanged)
	}
}

func TestRenderValNull(t *testing.T) {
	got := renderVal(json.RawMessage("null"))
	if got != "(null)" {
		t.Errorf("renderVal(null) = %q, want \"(null)\"", got)
	}
}

func TestDiffAttrsNullValues(t *testing.T) {
	// A change where before and after are both JSON null for key "tags".
	ch := tfChange{
		Before: json.RawMessage(`{"tags": null}`),
		After:  json.RawMessage(`{"tags": null}`),
	}
	attrs, _ := diffAttrs(ch)
	// null == null → unchanged, should not appear in attrs
	for _, a := range attrs {
		if a.Path == "tags" {
			t.Errorf("tags is unchanged (null==null) but appeared as %+v", a)
		}
	}

	// Change from null to a real value.
	ch2 := tfChange{
		Before: json.RawMessage(`{"tags": null}`),
		After:  json.RawMessage(`{"tags": "prod"}`),
	}
	attrs2, _ := diffAttrs(ch2)
	a, ok := find(attrs2, "tags")
	if !ok {
		t.Fatal("tags missing from diff")
	}
	if a.Before != "(null)" {
		t.Errorf("tags.Before = %q, want \"(null)\"", a.Before)
	}
	if a.After != "prod" {
		t.Errorf("tags.After = %q, want \"prod\"", a.After)
	}
}

func TestDiffAttrsUnknownNoBeforeIsAttrAdd(t *testing.T) {
	// Unknown attribute with no before value (CREATE scenario): Kind must be AttrAdd.
	ch := tfChange{
		Before:       json.RawMessage(`{}`),
		After:        json.RawMessage(`{}`),
		AfterUnknown: json.RawMessage(`{"id": true}`),
	}
	attrs, _ := diffAttrs(ch)
	a, ok := find(attrs, "id")
	if !ok {
		t.Fatal("id missing from attrs")
	}
	if a.Kind != AttrAdd {
		t.Errorf("id.Kind = %v, want AttrAdd (create with unknown)", a.Kind)
	}
	if !a.Unknown {
		t.Error("id.Unknown should be true")
	}
}

func TestDiffAttrsUnknownWithBeforeIsAttrUpdate(t *testing.T) {
	// Unknown attribute WITH a before value (UPDATE scenario): Kind must be AttrUpdate.
	ch := tfChange{
		Before:       json.RawMessage(`{"id": "old-id"}`),
		After:        json.RawMessage(`{}`),
		AfterUnknown: json.RawMessage(`{"id": true}`),
	}
	attrs, _ := diffAttrs(ch)
	a, ok := find(attrs, "id")
	if !ok {
		t.Fatal("id missing from attrs")
	}
	if a.Kind != AttrUpdate {
		t.Errorf("id.Kind = %v, want AttrUpdate (update with unknown)", a.Kind)
	}
	if !a.Unknown {
		t.Error("id.Unknown should be true")
	}
}
