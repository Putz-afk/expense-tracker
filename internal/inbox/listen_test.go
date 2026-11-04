package inbox

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/store"
)

// step is one scripted answer of a fake mailbox.
type step struct {
	batch Batch
	err   error
}

// scriptedMailbox answers FetchSince from a list. When waitErrs runs out the
// next WaitForMail cancels the context, which ends Run.
type scriptedMailbox struct {
	fetches  []step
	waitErrs []error // result of each WaitForMail call, in order
	cancel   context.CancelFunc

	fetchCalls int
	waitCalls  int
}

func (m *scriptedMailbox) FetchSince(_ context.Context, _ store.SyncState) (Batch, error) {
	i := m.fetchCalls
	m.fetchCalls++
	if i >= len(m.fetches) {
		return Batch{UIDValidity: 1}, nil
	}
	return m.fetches[i].batch, m.fetches[i].err
}

func (m *scriptedMailbox) WaitForMail(ctx context.Context) error {
	i := m.waitCalls
	m.waitCalls++
	if i >= len(m.waitErrs) {
		m.cancel() // the script is over: simulate the user pressing Ctrl+C
		<-ctx.Done()
		return ctx.Err()
	}
	return m.waitErrs[i]
}

type report struct {
	res Result
	err error
}

func runListener(t *testing.T, m *scriptedMailbox, s *store.Store, sleeps *[]time.Duration) []report {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.cancel = cancel

	var reports []report
	l := &Listener{
		Source: m, Waiter: m, Store: s,
		Report:     func(r Result, err error) { reports = append(reports, report{r, err}) },
		MinBackoff: time.Second, MaxBackoff: 4 * time.Second,
		sleep: func(_ context.Context, d time.Duration) bool {
			*sleeps = append(*sleeps, d)
			return true
		},
	}
	if err := l.Run(ctx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return reports
}

func TestListenerSyncsAgainAfterEachWait(t *testing.T) {
	s := newStore(t)
	m := &scriptedMailbox{
		fetches: []step{
			{batch: Batch{UIDValidity: 1, Messages: []Message{{UID: 1, Raw: fixture(t, "qris.eml")}}}},
			{batch: Batch{UIDValidity: 1, Messages: []Message{{UID: 2, Raw: fixture(t, "transfer_bifast.eml")}}}},
		},
		waitErrs: []error{nil}, // first wait returns "new mail"; the second one is Ctrl+C
	}
	var sleeps []time.Duration

	reports := runListener(t, m, s, &sleeps)

	if len(reports) != 2 || reports[0].res.Added != 1 || reports[1].res.Added != 1 {
		t.Errorf("reports = %+v, want two syncs with one new item each", reports)
	}
	if pending, _ := s.ListPending(); len(pending) != 2 {
		t.Errorf("pending count = %d, want 2", len(pending))
	}
	if len(sleeps) != 0 {
		t.Errorf("slept %v, want no waiting after successful syncs", sleeps)
	}
}

func TestListenerRetriesAfterErrorsWithGrowingDelay(t *testing.T) {
	s := newStore(t)
	boom := errors.New("network down")
	m := &scriptedMailbox{
		fetches: []step{
			{err: boom}, {err: boom}, {err: boom}, {err: boom}, // four failures in a row
			{batch: Batch{UIDValidity: 1, Messages: []Message{{UID: 1, Raw: fixture(t, "qris.eml")}}}},
		},
		waitErrs: []error{nil},
	}
	var sleeps []time.Duration

	reports := runListener(t, m, s, &sleeps)

	// 1s, 2s, then capped at 4s.
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second}
	if !slices.Equal(sleeps, want) {
		t.Errorf("delays = %v, want %v", sleeps, want)
	}
	failures := 0
	for _, r := range reports {
		if errors.Is(r.err, boom) {
			failures++
		}
	}
	if failures != 4 {
		t.Errorf("reported %d failures, want 4 (reports: %+v)", failures, reports)
	}
	if pending, _ := s.ListPending(); len(pending) != 1 {
		t.Errorf("pending count = %d, want 1 (the listener must recover)", len(pending))
	}
}

func TestListenerBackoffResetsAfterSuccess(t *testing.T) {
	s := newStore(t)
	boom := errors.New("network down")
	m := &scriptedMailbox{
		fetches:  []step{{err: boom}, {err: boom}, {batch: Batch{UIDValidity: 1}}, {err: boom}},
		waitErrs: []error{nil}, // success, wait, then the next sync fails again
	}
	var sleeps []time.Duration

	runListener(t, m, s, &sleeps)

	// 1s, 2s while failing; healthy once; then back to 1s, not 4s.
	want := []time.Duration{time.Second, 2 * time.Second, time.Second}
	if !slices.Equal(sleeps, want) {
		t.Errorf("delays = %v, want %v", sleeps, want)
	}
}

func TestListenerReportsWaitFailureAndRetries(t *testing.T) {
	s := newStore(t)
	boom := errors.New("idle failed")
	m := &scriptedMailbox{
		fetches:  []step{{batch: Batch{UIDValidity: 1}}, {batch: Batch{UIDValidity: 1}}},
		waitErrs: []error{boom},
	}
	var sleeps []time.Duration

	reports := runListener(t, m, s, &sleeps)

	found := false
	for _, r := range reports {
		if errors.Is(r.err, boom) {
			found = true
		}
	}
	if !found {
		t.Errorf("wait failure was not reported: %+v", reports)
	}
	if len(sleeps) != 1 || sleeps[0] != time.Second {
		t.Errorf("delays = %v, want one 1s delay", sleeps)
	}
	if m.fetchCalls != 2 {
		t.Errorf("fetch calls = %d, want 2 (sync again after the failed wait)", m.fetchCalls)
	}
}

func TestListenerStopsWhenCancelledDuringBackoff(t *testing.T) {
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	m := &scriptedMailbox{fetches: []step{{err: errors.New("down")}}, cancel: cancel}

	l := &Listener{
		Source: m, Waiter: m, Store: s,
		Report: func(Result, error) {},
		// The sleep reports "interrupted", like sleepContext does on Ctrl+C.
		sleep: func(context.Context, time.Duration) bool { cancel(); return false },
	}
	if err := l.Run(ctx); err != nil {
		t.Errorf("Run() error = %v, want nil on cancellation", err)
	}
}

func TestSleepContext(t *testing.T) {
	if !sleepContext(context.Background(), time.Millisecond) {
		t.Error("sleepContext() = false, want true when the time passes")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if sleepContext(ctx, time.Hour) {
		t.Error("sleepContext() = true, want false when cancelled")
	}
	if time.Since(start) > time.Second {
		t.Error("sleepContext() did not return promptly after cancellation")
	}
}
