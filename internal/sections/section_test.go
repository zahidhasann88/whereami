package sections

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zahidhasann88/whereami/internal/env"
)

type fakeSection struct {
	name string
	fn   func(ctx context.Context) (Result, error)
}

func (f fakeSection) Name() string { return f.name }
func (f fakeSection) Collect(ctx context.Context, _ *env.Env) (Result, error) {
	return f.fn(ctx)
}

func TestSectionIsolation(t *testing.T) {
	secs := []Section{
		fakeSection{"good-before", func(context.Context) (Result, error) {
			return Result{Status: StatusOK, Data: "a"}, nil
		}},
		fakeSection{"erroring", func(context.Context) (Result, error) {
			return Result{}, errors.New("disk on fire")
		}},
		fakeSection{"panicking", func(context.Context) (Result, error) {
			panic("nil map write")
		}},
		fakeSection{"slow", func(ctx context.Context) (Result, error) {
			select {
			case <-time.After(10 * time.Second):
				return Result{Status: StatusOK}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}},
		fakeSection{"good-after", func(context.Context) (Result, error) {
			return Result{Status: StatusOK, Data: "b"}, nil
		}},
	}
	start := time.Now()
	results := Run(context.Background(), &env.Env{Dir: "."}, secs, 100*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Fatalf("Run took %s; the slow section should have been cut off at 100ms", elapsed)
	}
	want := []struct {
		name   string
		status Status
	}{
		{"good-before", StatusOK},
		{"erroring", StatusUnavailable},
		{"panicking", StatusUnavailable},
		{"slow", StatusTimedOut},
		{"good-after", StatusOK},
	}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d", len(results), len(want))
	}
	for i, w := range want {
		if results[i].Name != w.name || results[i].Status != w.status {
			t.Errorf("result %d = %s/%s, want %s/%s (%s)", i, results[i].Name, results[i].Status, w.name, w.status, results[i].Reason)
		}
	}
	if results[1].Reason != "disk on fire" {
		t.Errorf("error reason = %q", results[1].Reason)
	}
	if results[2].Reason == "" {
		t.Error("panic should carry a reason")
	}
	if results[3].Reason == "" {
		t.Error("timeout should carry a reason")
	}
	if results[4].Data != "b" {
		t.Errorf("good section data lost: %+v", results[4])
	}
}

func TestRegistryRejectsDuplicates(t *testing.T) {
	a := fakeSection{"x", nil}
	if _, err := NewRegistry(a, a); err == nil {
		t.Error("expected duplicate error")
	}
}

func TestBuiltinRegistryOrderAndLookup(t *testing.T) {
	r := Builtin()
	want := []string{"project", "git", "runtime", "ports", "containers", "config"}
	got := r.Names()
	if len(got) != len(want) {
		t.Fatalf("names = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("names[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	if _, ok := r.Lookup("ports"); !ok {
		t.Error("ports not found")
	}
	if _, ok := r.Lookup("nope"); ok {
		t.Error("unexpected section")
	}
}
