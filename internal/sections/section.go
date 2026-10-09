// Package sections collects independent project summaries.
package sections

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Status describes the outcome of one section.
type Status string

const (
	// StatusOK means the section ran and Data is populated.
	StatusOK Status = "ok"
	// StatusUnavailable means the data could not be obtained; Reason says why.
	StatusUnavailable Status = "unavailable"
	// StatusTimedOut means the section exceeded its time budget.
	StatusTimedOut Status = "timed_out"
	// StatusSkipped hides sections with nothing to show from text output.
	StatusSkipped Status = "skipped"
)

// Result carries section data or an unavailable, timed-out or skipped status.
type Result struct {
	Name   string
	Status Status
	Reason string
	Data   any
}

// Section collects its own data and determines whether it applies.
type Section interface {
	Name() string
	Collect(ctx context.Context, e *env.Env) (Result, error)
}

// Registry holds the sections available to the CLI, in display order.
type Registry struct {
	order []Section
	byKey map[string]Section
}

// NewRegistry builds a registry. Names must be unique.
func NewRegistry(secs ...Section) (*Registry, error) {
	r := &Registry{byKey: make(map[string]Section, len(secs))}
	for _, s := range secs {
		if _, dup := r.byKey[s.Name()]; dup {
			return nil, fmt.Errorf("duplicate section name %q", s.Name())
		}
		r.byKey[s.Name()] = s
		r.order = append(r.order, s)
	}
	return r, nil
}

// All returns every section in display order.
func (r *Registry) All() []Section {
	out := make([]Section, len(r.order))
	copy(out, r.order)
	return out
}

// Lookup returns the section with the given name.
func (r *Registry) Lookup(name string) (Section, bool) {
	s, ok := r.byKey[name]
	return s, ok
}

// Names returns the section names in display order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.order))
	for _, s := range r.order {
		names = append(names, s.Name())
	}
	return names
}

// Builtin returns the registry of built-in sections.
func Builtin() *Registry {
	r, err := NewRegistry(
		Project{},
		Git{},
		Runtime{},
		Ports{},
		Containers{},
		Config{},
	)
	if err != nil {
		// Built-in names are constants; a duplicate is a programming error.
		panic(fmt.Sprintf("invalid built-in registry: %v", err))
	}
	return r
}

// Run collects sections concurrently with individual timeouts, preserving input order.
func Run(ctx context.Context, e *env.Env, secs []Section, timeout time.Duration) []Result {
	results := make([]Result, len(secs))
	done := make(chan struct{}, len(secs))
	for i, s := range secs {
		go func(i int, s Section) {
			results[i] = runOne(ctx, s, e, timeout)
			done <- struct{}{}
		}(i, s)
	}
	for range secs {
		<-done
	}
	return results
}

func runOne(parent context.Context, s Section, e *env.Env, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	ch := make(chan Result, 1)
	go func() {
		ch <- collectSafely(ctx, s, e)
	}()

	select {
	case r := <-ch:
		return r
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Result{
				Name:   s.Name(),
				Status: StatusTimedOut,
				Reason: fmt.Sprintf("timed out after %s", timeout),
			}
		}
		return Result{Name: s.Name(), Status: StatusUnavailable, Reason: "canceled"}
	}
}

// collectSafely converts errors and panics from Collect into Results.
func collectSafely(ctx context.Context, s Section, e *env.Env) (res Result) {
	defer func() {
		if p := recover(); p != nil {
			res = Result{Name: s.Name(), Status: StatusUnavailable, Reason: fmt.Sprintf("internal error: %v", p)}
		}
	}()
	r, err := s.Collect(ctx, e)
	if err != nil {
		return Result{Name: s.Name(), Status: StatusUnavailable, Reason: err.Error()}
	}
	r.Name = s.Name()
	if r.Status == "" {
		r.Status = StatusOK
	}
	return r
}
