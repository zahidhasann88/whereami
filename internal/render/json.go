package render

import (
	"encoding/json"
	"io"
	"time"

	"github.com/zahidhasann88/whereami/internal/sections"
)

// JSONReport is the stable, documented JSON schema (see README "JSON output").
type JSONReport struct {
	SchemaVersion int           `json:"schema_version"`
	Tool          string        `json:"tool"`
	Version       string        `json:"version"`
	GeneratedAt   string        `json:"generated_at"`
	Path          string        `json:"path"`
	Sections      []JSONSection `json:"sections"`
}

// JSONSection includes data only when status is ok.
type JSONSection struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// BuildJSON converts a report into the JSON schema structure.
func BuildJSON(rep Report) JSONReport {
	out := JSONReport{
		SchemaVersion: SchemaVersion,
		Tool:          "whereami",
		Version:       rep.Version,
		GeneratedAt:   rep.GeneratedAt.UTC().Format(time.RFC3339),
		Path:          rep.Path,
		Sections:      []JSONSection{},
	}
	for _, r := range rep.Results {
		js := JSONSection{Name: r.Name, Status: string(r.Status), Reason: r.Reason}
		if r.Status == sections.StatusOK {
			js.Data = r.Data
		}
		out.Sections = append(out.Sections, js)
	}
	return out
}

// JSON writes the report as indented JSON.
func JSON(w io.Writer, rep Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(BuildJSON(rep))
}
