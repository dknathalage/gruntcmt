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

// hasRenderable reports whether the unit has at least one change that would
// be written into the diff block (i.e. not NoOp or Read).
func hasRenderable(u plan.Unit) bool {
	for _, c := range u.Changes {
		if c.Action != plan.ActionNoOp && c.Action != plan.ActionRead {
			return true
		}
	}
	return false
}

// groupStatusCell returns the status cell string for a group row.
func groupStatusCell(g analyze.Group) string {
	if g.Counts.Destroy > 0 {
		return "⚠️ **destroys**"
	}
	if g.Severity == 0 {
		return "➖ no-op"
	}
	return "✅"
}

// unitSeverity returns the max severity across all changes in a unit.
func unitSeverity(u plan.Unit) int {
	sev := 0
	for _, c := range u.Changes {
		if s := c.Action.Severity(); s > sev {
			sev = s
		}
	}
	return sev
}

// Marker returns the HTML comment gruntcmt emits as the first output line. It is
// the stable per-scope identity used for update-in-place; callers that post the
// comment themselves (e.g. --out gh) match on this exact string.
func Marker(scope string) string {
	return fmt.Sprintf("<!-- gruntcmt:scope=%s -->", scope)
}

func Render(r analyze.Report, s config.Settings) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", Marker(r.Scope))
	title := r.Title
	if title == "" {
		title = "Terragrunt plan"
	}

	// Compute total unit count across all groups.
	totalUnits := 0
	for _, g := range r.Groups {
		totalUnits += len(g.Units)
	}

	// Finding 1: headline includes N units.
	fmt.Fprintf(&b, "### %s %s — `%s` · %d units · %d destroy · %d add · %d change\n\n",
		headEmoji(r, s), title, r.Scope, totalUnits, r.Totals.Destroy, r.Totals.Add, r.Totals.Change)

	// Finding 2: summary table with trailing status column (6 columns).
	b.WriteString("| Group | Units | Add | Change | Destroy | |\n|---|---|---|---|---|---|\n")
	for _, g := range r.Groups {
		key := g.Key
		if key == "" {
			key = "(all)"
		}
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %s |\n",
			key, len(g.Units), g.Counts.Add, g.Counts.Change, g.Counts.Destroy, groupStatusCell(g))
	}
	b.WriteString("\n")

	if r.Totals.Destroy > 0 {
		noun := "changes"
		if r.Totals.Destroy == 1 {
			noun = "change"
		}
		fmt.Fprintf(&b, "⚠️ **%d destructive %s** — review carefully.\n\n", r.Totals.Destroy, noun)
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
	// NOTE: The blank lines surrounding inner/outer <details> tags are required
	// for GitHub to render nested folds as block-level elements, not inline text.
	anyDetail := false
	for _, u := range g.Units {
		if u.Detail != plan.FidelitySummary {
			anyDetail = true
		}
	}
	if !anyDetail {
		return
	}

	// Finding 3: fold-noop — entire no-op group renders as a collapsed one-liner.
	if s.Render.FoldNoop && g.Severity == 0 {
		fmt.Fprintf(b, "<details><summary>➖ <code>%s</code> — %d units · no changes</summary>\n\nNo changes.\n\n</details>\n\n", g.Key, len(g.Units))
		return
	}

	fmt.Fprintf(b, "<details><summary><code>%s</code> — %d units</summary>\n\n", g.Key, len(g.Units))
	for _, u := range g.Units {
		if u.Detail == plan.FidelitySummary {
			continue
		}

		// Finding 3: fold-noop — no-op unit renders as a collapsed one-liner.
		if s.Render.FoldNoop && unitSeverity(u) == 0 {
			fmt.Fprintf(b, "<details><summary><code>%s</code> — no changes</summary>\n\n</details>\n\n", u.Name)
			continue
		}

		// If every change is no-op/read, emit a "no changes" one-liner instead
		// of an empty diff block (regardless of FoldNoop setting).
		if !hasRenderable(u) {
			fmt.Fprintf(b, "<details><summary><code>%s</code> — no changes</summary>\n\nNo changes.\n\n</details>\n\n", u.Name)
			continue
		}

		fmt.Fprintf(b, "<details><summary><code>%s</code></summary>\n\n```diff\n", u.Name)
		for _, c := range u.Changes {
			// Finding 4: skip no-op and read resources (they are noise).
			if c.Action == plan.ActionNoOp || c.Action == plan.ActionRead {
				continue
			}
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
	glyph := "~"
	switch a.Kind {
	case plan.AttrAdd:
		glyph = "+"
	case plan.AttrRemove:
		glyph = "-"
	}
	fmt.Fprintf(b, "    %s %s = %s%s\n", glyph, a.Path, val, suffix)
}
