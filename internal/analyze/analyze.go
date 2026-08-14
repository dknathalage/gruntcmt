package analyze

import (
	"sort"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/config"
	"github.com/dknathalage/gruntcmt/internal/plan"
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

func Analyze(units []plan.Unit, loadErrs []plan.LoadError, s config.Settings) Report {
	r := Report{Scope: s.Scope, Title: s.Render.Title, Commit: s.Commit, LoadErrors: loadErrs}
	if r.Title == "" {
		r.Title = "Terragrunt plan"
	}
	byKey := map[string]*Group{}
	var order []string
	for _, u := range units {
		u.Detail = s.DetailFor(u.Name)
		if u.TerraformVersion != "" {
			r.TerraformVersion = u.TerraformVersion
		}
		key := groupKey(u.Name, s.GroupBy)
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
