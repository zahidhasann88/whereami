package sections

import (
	"encoding/json"
	"fmt"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

// exitCodeErr is a plain error used to stand in for a git exit status in tests.
type exitCodeErr int

func (e exitCodeErr) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// ExitCode mirrors *exec.ExitError.
func (e exitCodeErr) ExitCode() int { return int(e) }
