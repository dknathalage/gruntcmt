package analyze

import (
	"sort"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/plan"
	"github.com/dknathalage/gruntcmt/internal/ruleset"
)

type Group struct {
	Key      string
	Units    []plan.Unit
	Counts   plan.Counts
	Severity int
}

type Report struct {
	Scope            string
	Title            string
	GroupBy          int
	TerraformVersion string
	Commit           string
	Groups           []Group
	LoadErrors       []plan.LoadError
	Totals           plan.Counts
	Severity         int
}

func groupKey(unitPath string, n int) string {
	if n <= 0 {
		return ""
	}
	segs := strings.Split(unitPath, "/")
	if len(segs) <= n {
		return unitPath
	}
	return strings.Join(segs[:n], "/")
}

func unitSeverity(u plan.Unit) int {
	sev := 0
	for _, c := range u.Changes {
		if s := c.Action.Severity(); s > sev {
			sev = s
		}
	}
	return sev
}

func addCounts(a *plan.Counts, b plan.Counts) {
	a.Add += b.Add
	a.Change += b.Change
	a.Destroy += b.Destroy
	a.Replace += b.Replace
	a.NoOp += b.NoOp
}

func buildReport(scope, title string, groupBy int, units []plan.Unit, loadErrs []plan.LoadError) Report {
	r := Report{Scope: scope, Title: title, GroupBy: groupBy, LoadErrors: loadErrs}
	byKey := map[string]*Group{}
	var order []string
	for _, u := range units {
		if r.TerraformVersion == "" && u.TerraformVersion != "" {
			r.TerraformVersion = u.TerraformVersion
		}
		key := groupKey(u.Name, groupBy)
		g, ok := byKey[key]
		if !ok {
			g = &Group{Key: key}
			byKey[key] = g
			order = append(order, key)
		}
		g.Units = append(g.Units, u)
		addCounts(&g.Counts, u.Counts)
		if sv := unitSeverity(u); sv > g.Severity {
			g.Severity = sv
		}
		addCounts(&r.Totals, u.Counts)
	}
	for _, key := range order {
		g := byKey[key]
		sort.SliceStable(g.Units, func(i, j int) bool {
			si, sj := unitSeverity(g.Units[i]), unitSeverity(g.Units[j])
			if si != sj {
				return si > sj
			}
			return g.Units[i].Name < g.Units[j].Name
		})
		if g.Severity > r.Severity {
			r.Severity = g.Severity
		}
		r.Groups = append(r.Groups, *g)
	}
	sort.SliceStable(r.Groups, func(i, j int) bool {
		if r.Groups[i].Severity != r.Groups[j].Severity {
			return r.Groups[i].Severity > r.Groups[j].Severity
		}
		return r.Groups[i].Key < r.Groups[j].Key
	})
	return r
}

func Analyze(units []plan.Unit, loadErrs []plan.LoadError, rs ruleset.Ruleset, mainScope string) []Report {
	type bucket struct {
		scope     string
		dedicated bool
		units     []plan.Unit
	}
	order := []string{mainScope}
	buckets := map[string]*bucket{mainScope: {scope: mainScope, dedicated: false}}
	for _, s := range rs.DedicatedScopes() {
		order = append(order, s)
		buckets[s] = &bucket{scope: s, dedicated: true}
	}
	for _, u := range units {
		// Defensive copy: make a fresh Changes slice so caller's backing array is never mutated
		changes := make([]plan.ResourceChange, len(u.Changes))
		copy(changes, u.Changes)
		for i := range changes {
			changes[i].Detail = rs.Detail(u.Name, changes[i].Action)
		}
		u.Changes = changes
		scope, dedicated := rs.Assign(u.Name)
		key := mainScope
		if dedicated {
			key = scope
		}
		b := buckets[key]
		if b == nil {
			b = buckets[mainScope]
		}
		b.units = append(b.units, u)
	}

	var reports []Report
	for _, key := range order {
		b := buckets[key]
		isMain := key == mainScope
		var le []plan.LoadError
		if isMain {
			le = loadErrs
		}
		if len(b.units) == 0 && len(le) == 0 {
			continue
		}
		reports = append(reports, buildReport(b.scope, rs.Title(b.scope, b.dedicated), rs.GroupBy(b.scope, b.dedicated), b.units, le))
	}
	return reports
}
