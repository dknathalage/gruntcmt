package render

import (
	"fmt"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/analyze"
	"github.com/dknathalage/gruntcmt/internal/plan"
)

// Built-in emoji constants — no override map.
const (
	emojiDestroy = "🔴"
	emojiChange  = "🟡"
	emojiAdd     = "🟢"
	emojiNoop    = "➖"
)

func headEmoji(r analyze.Report) string {
	switch {
	case r.Totals.Destroy > 0 || r.Totals.Replace > 0:
		return emojiDestroy
	case r.Totals.Change > 0:
		return emojiChange
	case r.Totals.Add > 0:
		return emojiAdd
	default:
		return emojiNoop
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

// Marker returns the HTML comment gruntcmt emits as the first output line. It is
// the stable per-scope identity used for update-in-place; callers that post the
// comment themselves (e.g. --out gh) match on this exact string.
func Marker(scope string) string {
	return fmt.Sprintf("<!-- gruntcmt:scope=%s -->", scope)
}

// Render produces the full Markdown comment body for a plan report.
// Per-change Detail fidelity controls what appears in the diff blocks:
//   - FidelitySummary changes are omitted from diff bodies (counted only in the table).
//   - FidelityResource changes emit address + action glyph only.
//   - FidelityAttribute changes emit address + attribute lines.
//
// A unit with no Changes at all (true no-op) renders as a folded "no changes" one-liner.
// A unit with changes but ALL at FidelitySummary renders nothing (table-only).
func Render(r analyze.Report) string {
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

	fmt.Fprintf(&b, "### %s %s — `%s` · %d units · %d destroy · %d add · %d change\n\n",
		headEmoji(r), title, r.Scope, totalUnits, r.Totals.Destroy, r.Totals.Add, r.Totals.Change)

	// Summary table with trailing status column.
	b.WriteString("| Group | Units | Add | Change | Destroy | |\n|---|---|---|---|---|---|\n")
	for _, g := range r.Groups {
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d | %d | %s |\n",
			groupLabel(g.Key), len(g.Units), g.Counts.Add, g.Counts.Change, g.Counts.Destroy, groupStatusCell(g))
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
		renderGroup(&b, g)
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

// unitBody renders the body for a single unit and returns it. Returns "" if
// the unit should be suppressed (has changes but all are FidelitySummary).
// Returns a "no changes" one-liner if the unit has no changes at all.
func unitBody(u plan.Unit) string {
	// True no-op: unit has no changes at all.
	if len(u.Changes) == 0 {
		return fmt.Sprintf("<details><summary><code>%s</code> — no changes</summary>\n\nNo changes.\n\n</details>\n\n", u.Name)
	}

	// Collect renderable changes (Detail != FidelitySummary).
	var renderable []plan.ResourceChange
	for _, c := range u.Changes {
		if c.Detail != plan.FidelitySummary {
			renderable = append(renderable, c)
		}
	}

	// All changes are summary-fidelity → render nothing (table-only).
	if len(renderable) == 0 {
		return ""
	}

	// Render the unit diff block with only renderable changes.
	var b strings.Builder
	fmt.Fprintf(&b, "<details><summary><code>%s</code></summary>\n\n```diff\n", u.Name)
	for _, c := range renderable {
		fmt.Fprintf(&b, "%s %s\n", actionGlyph(c.Action), c.Address)
		if c.Detail == plan.FidelityAttribute {
			for _, a := range c.Attributes {
				renderAttr(&b, a)
			}
			// Unchanged attributes are always omitted — no count line.
		}
	}
	b.WriteString("```\n</details>\n\n")
	return b.String()
}

// groupLabel is the display name for a group key; the empty key (group-by 0, a
// single flat group) renders as "(all)" in both the table and the group header.
func groupLabel(key string) string {
	if key == "" {
		return "(all)"
	}
	return key
}

func renderGroup(b *strings.Builder, g analyze.Group) {
	// Collect unit bodies; skip units that produce nothing (all-summary changes).
	var bodies []string
	for _, u := range g.Units {
		if body := unitBody(u); body != "" {
			bodies = append(bodies, body)
		}
	}
	// Group renders only if at least one unit produced a body.
	if len(bodies) == 0 {
		return
	}

	fmt.Fprintf(b, "<details><summary><code>%s</code> — %d units</summary>\n\n", groupLabel(g.Key), len(g.Units))
	for _, body := range bodies {
		b.WriteString(body)
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
