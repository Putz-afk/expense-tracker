package inbox

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Putz-afk/expense-tracker/internal/store"
)

// fakeSource returns a fixed batch and remembers the state it was asked with.
type fakeSource struct {
	batch Batch
	err   error
	got   []store.SyncState
}

func (f *fakeSource) FetchSince(_ context.Context, st store.SyncState) (Batch, error) {
	f.got = append(f.got, st)
	return f.batch, f.err
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(store.MemoryPath)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../bri/testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

const notATransaction = "From: Bank BRI <BankBRI@bri.co.id>\nSubject: OTP\nContent-Type: text/html\n\n<p>Kode OTP 123456</p>"
const unparsable = "From: Bank BRI <BankBRI@bri.co.id>\nSubject: x\nContent-Type: text/html\n\n<td>Total Transaksi</td><td>abc</td>"

func TestSync(t *testing.T) {
	s := newStore(t)
	src := &fakeSource{batch: Batch{
		UIDValidity: 7,
		Messages: []Message{
			{UID: 10, Raw: fixture(t, "qris.eml")},
			{UID: 11, Raw: []byte(notATransaction)},
			{UID: 12, Raw: fixture(t, "topup_dana.eml")},
			{UID: 13, Raw: []byte(unparsable)},
			{UID: 14, Raw: fixture(t, "transfer_bifast.eml")},
		},
	}}

	res, err := Sync(context.Background(), src, s)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}
	if res.Added != 3 || res.Duplicates != 0 || res.Skipped != 1 {
		t.Errorf("result = %+v, want 3 added, 0 duplicates, 1 skipped", res)
	}
	if len(res.Failed) != 1 || res.Failed[0].UID != 13 {
		t.Errorf("Failed = %+v, want one failure for UID 13", res.Failed)
	}

	pending, err := s.ListPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 3 {
		t.Fatalf("pending count = %d, want 3", len(pending))
	}
	// Oldest first: the transfer (24 Oct), then the QRIS and the top up (2 Nov).
	if pending[0].Kind != "Transfer BI-FAST" || pending[0].Category != "other" {
		t.Errorf("first pending = %+v", pending[0])
	}
	if pending[1].Kind != "QRIS Bayar" || pending[1].Category != "food" {
		t.Errorf("second pending = %+v, want QRIS suggested as food", pending[1])
	}

	st, err := s.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if want := (store.SyncState{UIDValidity: 7, LastUID: 14}); st != want {
		t.Errorf("sync state = %+v, want %+v", st, want)
	}
}

func TestSyncTwiceCreatesNoDuplicates(t *testing.T) {
	s := newStore(t)
	src := &fakeSource{batch: Batch{
		UIDValidity: 7,
		Messages:    []Message{{UID: 10, Raw: fixture(t, "qris.eml")}},
	}}

	if _, err := Sync(context.Background(), src, s); err != nil {
		t.Fatal(err)
	}
	res, err := Sync(context.Background(), src, s) // the same mail arrives again
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 0 || res.Duplicates != 1 {
		t.Errorf("second result = %+v, want 0 added, 1 duplicate", res)
	}
	if pending, _ := s.ListPending(); len(pending) != 1 {
		t.Errorf("pending count = %d, want 1", len(pending))
	}

	// The second call must have been told where the first one stopped.
	if want := (store.SyncState{UIDValidity: 7, LastUID: 10}); src.got[1] != want {
		t.Errorf("second FetchSince state = %+v, want %+v", src.got[1], want)
	}
}

func TestSyncUIDValidityChangeResetsPosition(t *testing.T) {
	s := newStore(t)
	if err := s.SaveSyncState(store.SyncState{UIDValidity: 7, LastUID: 500}); err != nil {
		t.Fatal(err)
	}
	// The server reset its numbering: new validity, low UIDs again.
	src := &fakeSource{batch: Batch{
		UIDValidity: 8,
		Messages:    []Message{{UID: 3, Raw: fixture(t, "qris.eml")}},
	}}

	if _, err := Sync(context.Background(), src, s); err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if want := (store.SyncState{UIDValidity: 8, LastUID: 3}); st != want {
		t.Errorf("sync state = %+v, want %+v (old UID 500 must be forgotten)", st, want)
	}
}

func TestSyncEmptyBatchKeepsPosition(t *testing.T) {
	s := newStore(t)
	if err := s.SaveSyncState(store.SyncState{UIDValidity: 7, LastUID: 500}); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{batch: Batch{UIDValidity: 7}}

	if _, err := Sync(context.Background(), src, s); err != nil {
		t.Fatal(err)
	}
	st, _ := s.LoadSyncState()
	if want := (store.SyncState{UIDValidity: 7, LastUID: 500}); st != want {
		t.Errorf("sync state = %+v, want %+v", st, want)
	}
}

func TestSyncSourceError(t *testing.T) {
	s := newStore(t)
	boom := errors.New("network down")
	src := &fakeSource{err: boom}

	_, err := Sync(context.Background(), src, s)
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want it to wrap %v", err, boom)
	}
	if !strings.Contains(err.Error(), "fetch mail") {
		t.Errorf("error = %q, want context 'fetch mail'", err)
	}
}

func TestSyncStopsWhenCancelled(t *testing.T) {
	s := newStore(t)
	src := &fakeSource{batch: Batch{
		UIDValidity: 7,
		Messages:    []Message{{UID: 10, Raw: fixture(t, "qris.eml")}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Sync(ctx, src, s)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	// Cancelled before saving: the position must not move.
	if st, _ := s.LoadSyncState(); st != (store.SyncState{}) {
		t.Errorf("sync state = %+v, want unchanged", st)
	}
}
