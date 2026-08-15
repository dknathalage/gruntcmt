package plan

import (
	"encoding/json"
	"sort"
)

func boolMap(raw json.RawMessage) map[string]bool {
	out := map[string]bool{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return out
	}
	for k, v := range m {
		var b bool
		if json.Unmarshal(v, &b) == nil && b {
			out[k] = true
		}
	}
	return out
}

func replaceKeys(raw json.RawMessage) map[string]bool {
	out := map[string]bool{}
	var paths [][]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &paths) != nil {
		return out
	}
	for _, p := range paths {
		if len(p) == 0 {
			continue
		}
		var key string
		if json.Unmarshal(p[0], &key) == nil {
			out[key] = true
		}
	}
	return out
}

func renderVal(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if string(raw) == "null" {
		return "(null)"
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func diffAttrs(ch tfChange) ([]AttributeChange, int) {
	before := map[string]json.RawMessage{}
	after := map[string]json.RawMessage{}
	_ = json.Unmarshal(ch.Before, &before)
	_ = json.Unmarshal(ch.After, &after)
	unknown := boolMap(ch.AfterUnknown)
	sens := boolMap(ch.AfterSensitive)
	for k := range boolMap(ch.BeforeSensitive) {
		sens[k] = true
	}
	forces := replaceKeys(ch.ReplacePaths)

	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	for k := range unknown {
		keys[k] = true
	}

	// Sort keys for deterministic attribute order.
	keySlice := make([]string, 0, len(keys))
	for k := range keys {
		keySlice = append(keySlice, k)
	}
	sort.Strings(keySlice)

	var attrs []AttributeChange
	unchanged := 0
	for _, k := range keySlice {
		b, hasB := before[k]
		a, hasA := after[k]
		isUnknown := unknown[k]
		bs, as := renderVal(b), renderVal(a)
		switch {
		case isUnknown:
			kind := AttrUpdate
			if !hasB {
				kind = AttrAdd
			}
			attrs = append(attrs, AttributeChange{Path: k, Before: bs, After: "", Kind: kind, Unknown: true, Sensitive: sens[k], ForcesNew: forces[k]})
		case hasB && !hasA:
			attrs = append(attrs, AttributeChange{Path: k, Before: bs, Kind: AttrRemove, Sensitive: sens[k], ForcesNew: forces[k]})
		case !hasB && hasA:
			attrs = append(attrs, AttributeChange{Path: k, After: as, Kind: AttrAdd, Sensitive: sens[k], ForcesNew: forces[k]})
		case string(b) == string(a):
			unchanged++
		default:
			attrs = append(attrs, AttributeChange{Path: k, Before: bs, After: as, Kind: AttrUpdate, Sensitive: sens[k], ForcesNew: forces[k]})
		}
	}
	return attrs, unchanged
}
