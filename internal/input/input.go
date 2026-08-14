package input

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/dknathalage/gruntcmt/internal/plan"
)

type Mode int

const (
	ModeAuto Mode = iota
	ModeWrapped
	ModePlan
)

type wrapped struct {
	Name string          `json:"name"`
	Plan json.RawMessage `json:"plan"`
}

// undecodableOuterRecord is the Name value assigned to a LoadError when the
// outer NDJSON record itself cannot be decoded (structural error).
const undecodableOuterRecord = "?"

func Read(r io.Reader, mode Mode, defaultName string) ([]plan.Unit, []plan.LoadError, error) {
	all, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read stdin: %w", err)
	}
	if len(bytes.TrimSpace(all)) == 0 {
		return nil, nil, fmt.Errorf("empty stdin: no plan JSON")
	}

	if mode == ModeAuto {
		var probe map[string]json.RawMessage
		dec := json.NewDecoder(bytes.NewReader(all))
		if dec.Decode(&probe) == nil {
			if _, isPlan := probe["format_version"]; isPlan {
				mode = ModePlan
			} else {
				mode = ModeWrapped
			}
		} else {
			mode = ModeWrapped
		}
	}

	if mode == ModePlan {
		u, err := plan.ParsePlan(defaultName, all)
		if err != nil {
			return nil, []plan.LoadError{{Name: defaultName, Message: err.Error()}}, nil
		}
		return []plan.Unit{u}, nil, nil
	}

	var units []plan.Unit
	var loadErrs []plan.LoadError
	dec := json.NewDecoder(bytes.NewReader(all))
	for {
		var w wrapped
		derr := dec.Decode(&w)
		if derr == io.EOF {
			break
		}
		if derr != nil {
			// Structural error in outer NDJSON record: the Decoder's position is
			// unrecoverable after a decode error, so we record the error and break
			// the stream to avoid looping or emitting garbage.
			loadErrs = append(loadErrs, plan.LoadError{Name: undecodableOuterRecord, Message: derr.Error()})
			break
		}
		u, perr := plan.ParsePlan(w.Name, w.Plan)
		if perr != nil {
			loadErrs = append(loadErrs, plan.LoadError{Name: w.Name, Message: perr.Error()})
			continue
		}
		units = append(units, u)
	}
	if len(units) == 0 && len(loadErrs) == 0 {
		return nil, nil, fmt.Errorf("no plans found on stdin")
	}
	return units, loadErrs, nil
}
