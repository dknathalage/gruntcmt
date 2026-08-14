package plan

import (
	"encoding/json"
	"fmt"
)

type Action int

const (
	ActionNoOp Action = iota
	ActionRead
	ActionCreate
	ActionUpdate
	ActionReplace
	ActionDelete
)

func (a Action) Severity() int {
	switch a {
	case ActionDelete, ActionReplace:
		return 5
	case ActionUpdate:
		return 3
	case ActionCreate:
		return 2
	case ActionRead:
		return 1
	default:
		return 0
	}
}

func (a Action) Destructive() bool { return a == ActionDelete || a == ActionReplace }

type Fidelity int

const (
	FidelitySummary Fidelity = iota
	FidelityResource
	FidelityAttribute
)

type Counts struct{ Add, Change, Destroy, Replace, NoOp int }

type AttrKind int

const (
	AttrUpdate AttrKind = iota
	AttrAdd
	AttrRemove
)

type AttributeChange struct {
	Path      string // top-level attribute key
	Before    string // rendered value ("" when adding)
	After     string // rendered value ("" when removing)
	Kind      AttrKind
	ForcesNew bool
	Sensitive bool
	Unknown   bool
}

type ResourceChange struct {
	Address    string
	Action     Action
	Attributes []AttributeChange
	Unchanged  int
}

type OutputChange struct {
	Name   string
	Action Action
}

type Unit struct {
	Name             string
	TerraformVersion string
	Detail           Fidelity
	Changes          []ResourceChange
	OutputChanges    []OutputChange
	Drift            []ResourceChange
	Counts           Counts
}

type LoadError struct{ Name, Message string }

// Terraform plan JSON shapes we read.
type tfChange struct {
	Actions         []string        `json:"actions"`
	Before          json.RawMessage `json:"before"`
	After           json.RawMessage `json:"after"`
	AfterUnknown    json.RawMessage `json:"after_unknown"`
	BeforeSensitive json.RawMessage `json:"before_sensitive"`
	AfterSensitive  json.RawMessage `json:"after_sensitive"`
	ReplacePaths    json.RawMessage `json:"replace_paths"`
}

type tfResourceChange struct {
	Address string   `json:"address"`
	Change  tfChange `json:"change"`
}

type tfPlan struct {
	FormatVersion    string                      `json:"format_version"`
	TerraformVersion string                      `json:"terraform_version"`
	ResourceChanges  []tfResourceChange          `json:"resource_changes"`
	ResourceDrift    []tfResourceChange          `json:"resource_drift"`
	OutputChanges    map[string]tfChange         `json:"output_changes"`
}

func deriveAction(actions []string) Action {
	switch {
	case len(actions) == 2:
		return ActionReplace // ["delete","create"] or ["create","delete"]
	case len(actions) == 1:
		switch actions[0] {
		case "create":
			return ActionCreate
		case "update":
			return ActionUpdate
		case "delete":
			return ActionDelete
		case "read":
			return ActionRead
		case "no-op":
			return ActionNoOp
		}
	}
	return ActionUpdate // unknown combo → neutral "changed"
}

func ParsePlan(name string, raw []byte) (Unit, error) {
	var p tfPlan
	if err := json.Unmarshal(raw, &p); err != nil {
		return Unit{}, fmt.Errorf("%s: %w", name, err)
	}
	if p.FormatVersion == "" {
		return Unit{}, fmt.Errorf("%s: not a terraform plan (no format_version)", name)
	}
	u := Unit{Name: name, TerraformVersion: p.TerraformVersion}
	for _, rc := range p.ResourceChanges {
		u.Changes = append(u.Changes, ResourceChange{Address: rc.Address, Action: deriveAction(rc.Change.Actions)})
	}
	for _, rc := range p.ResourceDrift {
		u.Drift = append(u.Drift, ResourceChange{Address: rc.Address, Action: deriveAction(rc.Change.Actions)})
	}
	for on, oc := range p.OutputChanges {
		u.OutputChanges = append(u.OutputChanges, OutputChange{Name: on, Action: deriveAction(oc.Actions)})
	}
	u.Counts = countChanges(u.Changes)
	return u, nil
}

func countChanges(changes []ResourceChange) Counts {
	var c Counts
	for _, rc := range changes {
		switch rc.Action {
		case ActionCreate:
			c.Add++
		case ActionUpdate:
			c.Change++
		case ActionDelete:
			c.Destroy++
		case ActionReplace:
			c.Add++
			c.Destroy++
			c.Replace++
		case ActionNoOp:
			c.NoOp++
		}
	}
	return c
}
