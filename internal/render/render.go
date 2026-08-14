package render

import (
	"fmt"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/analyze"
	"github.com/dknathalage/gruntcmt/internal/config"
	"github.com/dknathalage/gruntcmt/internal/plan"
)

func emoji(s config.Settings, key, def string) string {
	if v, ok := s.Render.Emoji[key]; ok {
		return v
	}
	return def
}

func headEmoji(r analyze.Report, s config.Settings) string {
	switch {
	case r.Totals.Destroy > 0 || r.Totals.Replace > 0:
		return emoji(s, "destroy", "🔴")
	case r.Totals.Change > 0:
		return emoji(s, "change", "🟡")
	case r.Totals.Add > 0:
		return emoji(s, "add", "🟢")
	default:
		return emoji(s, "noop", "➖")
	}
}

func actionGlyph(a plan.Action) string {
	switch a {
	case plan.ActionCreate:
		return "+"
	case plan.ActionUpdate:
		return "~"
	case plan.ActionDelete:
		return "-"
	case plan.ActionReplace:
		return "-/+"
	default:
		return " "
	}
}

func Render(r analyze.Report, s config.Settings) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- gruntcmt:scope=%s -->\n", r.Scope)
	title := r.Title
	if title == "" {
		title = "Terragrunt plan"
	}
	fmt.Fprintf(&b, "### %s %s — `%s` · %d destroy · %d add · %d change\n\n",
		headEmoji(r, s), title, r.Scope, r.Totals.Destroy, r.Totals.Add, r.Totals.Change)

	// summary table
	b.WriteString("| Group | Units | Add | Change | Destroy |\n|---|---|---|---|---|\n")
	for _, g := range r.Groups {
		key := g.Key
		if key == "" {
			key = "(all)"
		}
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d |\n",
			key, len(g.Units), g.Counts.Add, g.Counts.Change, g.Counts.Destroy)
	}
	b.WriteString("\n")

	if r.Totals.Destroy > 0 {
		fmt.Fprintf(&b, "⚠️ **%d destructive changes** — review carefully.\n\n", r.Totals.Destroy)
	}

	for _, g := range r.Groups {
		renderGroup(&b, g, s)
	}

	if len(r.LoadErrors) > 0 {
		fmt.Fprintf(&b, "<details><summary>⚠️ %d units failed to parse</summary>\n\n", len(r.LoadErrors))
		for _, le := range r.LoadErrors {
			fmt.Fprintf(&b, "- `%s`: %s\n", le.Name, le.Message)
		}
		b.WriteString("\n</details>\n\n")
	}

	fmt.Fprintf(&b, "<sub>gruntcmt · terraform %s", r.TerraformVersion)
	if r.Commit != "" {
		fmt.Fprintf(&b, " · commit %s", r.Commit)
	}
	b.WriteString("</sub>\n")
	return b.String()
}

func renderGroup(b *strings.Builder, g analyze.Group, s config.Settings) {
	// summary fidelity groups contribute only to the table.
	anyDetail := false
	for _, u := range g.Units {
		if u.Detail != plan.FidelitySummary {
			anyDetail = true
		}
	}
	if !anyDetail {
		return
	}
	fmt.Fprintf(b, "<details><summary><code>%s</code> — %d units</summary>\n\n", g.Key, len(g.Units))
	for _, u := range g.Units {
		if u.Detail == plan.FidelitySummary {
			continue
		}
		fmt.Fprintf(b, "<details><summary><code>%s</code></summary>\n\n```diff\n", u.Name)
		for _, c := range u.Changes {
			fmt.Fprintf(b, "%s %s\n", actionGlyph(c.Action), c.Address)
			if u.Detail == plan.FidelityAttribute {
				for _, a := range c.Attributes {
					renderAttr(b, a)
				}
				if s.Render.HideUnchanged && c.Unchanged > 0 {
					fmt.Fprintf(b, "    # (%d unchanged attributes hidden)\n", c.Unchanged)
				}
			}
		}
		b.WriteString("```\n</details>\n\n")
	}
	b.WriteString("</details>\n\n")
}

func renderAttr(b *strings.Builder, a plan.AttributeChange) {
	val := func() string {
		switch {
		case a.Unknown:
			return "(known after apply)"
		case a.Sensitive:
			return "(sensitive value)"
		case a.Kind == plan.AttrAdd:
			return a.After
		case a.Kind == plan.AttrRemove:
			return a.Before
		default:
			return a.Before + " -> " + a.After
		}
	}()
	suffix := ""
	if a.ForcesNew {
		suffix = "  # forces replacement"
	}
	fmt.Fprintf(b, "    ~ %s = %s%s\n", a.Path, val, suffix)
}
