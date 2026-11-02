package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/store"
)

// testNow is the fixed clock used by command tests.
var testNow = time.Date(2026, time.November, 2, 23, 30, 0, 0, time.UTC)

// testEnv is a throwaway database file plus a fixed clock. Every command run
// through it opens the same file, just like separate runs of the real program.
type testEnv struct {
	t    *testing.T
	path string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return &testEnv{t: t, path: filepath.Join(t.TempDir(), "test.db")}
}

func (e *testEnv) deps() Deps {
	return Deps{
		OpenStore: func() (*store.Store, error) { return store.Open(e.path) },
		Now:       func() time.Time { return testNow },
	}
}

// run executes the CLI with args and returns everything it printed.
// Standard input is empty, so a command that asks a question gets no answer.
func (e *testEnv) run(args ...string) (string, error) {
	e.t.Helper()
	return e.runWithInput("", args...)
}

// runWithInput is like run but feeds input to the command as typed text.
func (e *testEnv) runWithInput(input string, args ...string) (string, error) {
	e.t.Helper()
	var out bytes.Buffer
	cmd := NewRootCmd(e.deps())
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// store opens the test database directly, for checking what commands saved.
func (e *testEnv) store() *store.Store {
	e.t.Helper()
	s, err := store.Open(e.path)
	if err != nil {
		e.t.Fatalf("open test store: %v", err)
	}
	e.t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRootCmdHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args shows help", []string{}},
		{"--help shows help", []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			out, err := env.run(tt.args...)
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			assertContains(t, out, "records your spending")
			assertContains(t, out, "add")
		})
	}
}
