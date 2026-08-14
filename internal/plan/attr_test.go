package plan

import "testing"

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
