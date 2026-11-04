package mail

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/Putz-afk/expense-tracker/internal/inbox"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

// These tests run the real Client against go-imap's in-memory IMAP server on
// a local TLS port, so the search, fetch and IDLE code is exercised without
// any account or network.

const (
	testUser     = "me@example.com"
	testPassword = "app-password"
)

// testServer is a running fake IMAP server with one user and an empty INBOX.
type testServer struct {
	addr string
	user *imapmemserver.User
	tls  *tls.Config // what a client needs to trust the server
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	// A self-signed certificate for 127.0.0.1, valid for an hour.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	mem := imapmemserver.New()
	user := imapmemserver.NewUser(testUser, testPassword)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
	})
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	return &testServer{
		addr: ln.Addr().String(),
		user: user,
		tls:  &tls.Config{RootCAs: pool},
	}
}

func (s *testServer) client(since time.Time) *Client {
	return New(Config{
		Addr: s.addr, Username: testUser, Password: testPassword,
		TLSConfig: s.tls, Since: since,
	})
}

// deliver puts a raw email into the INBOX, like the mail server receiving it.
func (s *testServer) deliver(t *testing.T, raw string) {
	t.Helper()
	// strings.Reader has Read and Size, which is all a LiteralReader needs.
	// The in-memory server needs non-nil options; empty ones mean "received now".
	if _, err := s.user.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("../bri/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func otherSender(from string) string {
	return "From: " + from + "\r\nSubject: hello\r\nContent-Type: text/plain\r\n\r\nhi"
}

// fillInbox delivers: UID 1 BRI QRIS, 2 a stranger, 3 a look-alike sender, 4 BRI DANA.
func fillInbox(t *testing.T, s *testServer) {
	t.Helper()
	s.deliver(t, fixture(t, "qris.eml"))
	s.deliver(t, otherSender("Someone <someone@example.com>"))
	s.deliver(t, otherSender("Bank BRI <bankbri@bri.co.id.evil.example>"))
	s.deliver(t, fixture(t, "topup_dana.eml"))
}

func uids(b inbox.Batch) []uint32 {
	var out []uint32
	for _, m := range b.Messages {
		out = append(out, m.UID)
	}
	return out
}

func TestFetchSinceFiltersBySender(t *testing.T) {
	s := newTestServer(t)
	fillInbox(t, s)

	batch, err := s.client(time.Time{}).FetchSince(context.Background(), store.SyncState{})
	if err != nil {
		t.Fatalf("FetchSince() error: %v", err)
	}
	if want := []uint32{1, 4}; !slices.Equal(uids(batch), want) {
		t.Errorf("UIDs = %v, want %v (strangers and look-alikes excluded, oldest first)", uids(batch), want)
	}
	if batch.UIDValidity == 0 {
		t.Error("UIDValidity = 0, want the mailbox's value")
	}
	// IMAP servers hand headers back with \r\n line endings; the fixture file
	// uses \n, so compare with the endings normalised.
	got := strings.ReplaceAll(string(batch.Messages[0].Raw), "\r\n", "\n")
	if got != fixture(t, "qris.eml") {
		t.Errorf("message 1 differs from what was delivered:\n%q", got)
	}
}

func TestFetchSinceContinuesAfterLastUID(t *testing.T) {
	s := newTestServer(t)
	fillInbox(t, s)
	c := s.client(time.Time{})

	first, err := c.FetchSince(context.Background(), store.SyncState{})
	if err != nil {
		t.Fatal(err)
	}
	pos := store.SyncState{UIDValidity: first.UIDValidity, LastUID: 1}

	batch, err := c.FetchSince(context.Background(), pos)
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint32{4}; !slices.Equal(uids(batch), want) {
		t.Errorf("after UID 1: UIDs = %v, want %v", uids(batch), want)
	}

	// Nothing is newer than UID 4. IMAP's "5:*" would still return message 4;
	// the client must filter that out.
	pos.LastUID = 4
	batch, err = c.FetchSince(context.Background(), pos)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Messages) != 0 {
		t.Errorf("after the newest UID: UIDs = %v, want none", uids(batch))
	}
	if batch.UIDValidity != first.UIDValidity {
		t.Errorf("UIDValidity = %d, want %d", batch.UIDValidity, first.UIDValidity)
	}

	// New mail shows up on the next call.
	s.deliver(t, fixture(t, "transfer_bifast.eml"))
	batch, err = c.FetchSince(context.Background(), pos)
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint32{5}; !slices.Equal(uids(batch), want) {
		t.Errorf("after delivery: UIDs = %v, want %v", uids(batch), want)
	}
}

func TestFetchSinceStartsOverWhenUIDValidityChanged(t *testing.T) {
	s := newTestServer(t)
	fillInbox(t, s)

	stale := store.SyncState{UIDValidity: 12345, LastUID: 4}
	batch, err := s.client(time.Time{}).FetchSince(context.Background(), stale)
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint32{1, 4}; !slices.Equal(uids(batch), want) {
		t.Errorf("UIDs = %v, want %v (saved position must be ignored)", uids(batch), want)
	}
}

func TestFetchSinceFirstSyncHonoursSince(t *testing.T) {
	s := newTestServer(t)
	fillInbox(t, s) // all delivered "now"

	tomorrow := time.Now().AddDate(0, 0, 2)
	batch, err := s.client(tomorrow).FetchSince(context.Background(), store.SyncState{})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Messages) != 0 {
		t.Errorf("with Since in the future: UIDs = %v, want none", uids(batch))
	}

	lastWeek := time.Now().AddDate(0, 0, -7)
	batch, err = s.client(lastWeek).FetchSince(context.Background(), store.SyncState{})
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint32{1, 4}; !slices.Equal(uids(batch), want) {
		t.Errorf("with Since last week: UIDs = %v, want %v", uids(batch), want)
	}
}

func TestFetchSinceLeavesMailUnread(t *testing.T) {
	s := newTestServer(t)
	fillInbox(t, s)

	if _, err := s.client(time.Time{}).FetchSince(context.Background(), store.SyncState{}); err != nil {
		t.Fatal(err)
	}

	// Look at the flags with a separate connection.
	cl, err := imapclient.DialTLS(s.addr, &imapclient.Options{TLSConfig: s.tls})
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if err := cl.Login(testUser, testPassword).Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.Select("INBOX", nil).Wait(); err != nil {
		t.Fatal(err)
	}
	msgs, err := cl.Fetch(imap.UIDSetNum(1, 4), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if slices.Contains(m.Flags, imap.FlagSeen) {
			t.Errorf("UID %d was marked \\Seen by fetching", m.UID)
		}
	}
}

func TestFetchSinceErrors(t *testing.T) {
	s := newTestServer(t)

	t.Run("wrong password", func(t *testing.T) {
		c := New(Config{Addr: s.addr, Username: testUser, Password: "nope", TLSConfig: s.tls})
		_, err := c.FetchSince(context.Background(), store.SyncState{})
		if err == nil || !strings.Contains(err.Error(), "log in") {
			t.Errorf("error = %v, want a login error", err)
		}
	})

	t.Run("no such mailbox", func(t *testing.T) {
		c := New(Config{Addr: s.addr, Username: testUser, Password: testPassword, TLSConfig: s.tls, Mailbox: "Nope"})
		_, err := c.FetchSince(context.Background(), store.SyncState{})
		if err == nil || !strings.Contains(err.Error(), `open mailbox "Nope"`) {
			t.Errorf("error = %v, want a mailbox error", err)
		}
	})

	t.Run("server not reachable", func(t *testing.T) {
		c := New(Config{Addr: "127.0.0.1:1", Username: testUser, Password: testPassword, TLSConfig: s.tls})
		_, err := c.FetchSince(context.Background(), store.SyncState{})
		if err == nil || !strings.Contains(err.Error(), "connect to") {
			t.Errorf("error = %v, want a connection error", err)
		}
	})

	t.Run("untrusted certificate", func(t *testing.T) {
		c := New(Config{Addr: s.addr, Username: testUser, Password: testPassword}) // system roots
		if _, err := c.FetchSince(context.Background(), store.SyncState{}); err == nil {
			t.Error("connected to a server with an unknown certificate")
		}
	})

	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := s.client(time.Time{}).FetchSince(ctx, store.SyncState{})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	})
}

func TestWaitForMailReturnsWhenMailArrives(t *testing.T) {
	s := newTestServer(t)
	c := s.client(time.Time{})

	done := make(chan error, 1)
	go func() { done <- c.WaitForMail(context.Background()) }()

	// We can't tell when the client is ready (IDLE running), so keep
	// delivering until it reacts. Each delivery that lands during IDLE wakes it.
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("WaitForMail() error: %v", err)
			}
			return
		case <-tick.C:
			s.deliver(t, fixture(t, "qris.eml"))
		case <-deadline:
			t.Fatal("WaitForMail() did not return after new mail arrived")
		}
	}
}

func TestWaitForMailStopsOnCancel(t *testing.T) {
	s := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- s.client(time.Time{}).WaitForMail(ctx) }()

	time.Sleep(300 * time.Millisecond) // let it connect and start IDLE
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("WaitForMail() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForMail() did not return after cancellation")
	}
}

func TestWaitForMailErrors(t *testing.T) {
	s := newTestServer(t)
	c := New(Config{Addr: s.addr, Username: testUser, Password: "nope", TLSConfig: s.tls})
	if err := c.WaitForMail(context.Background()); err == nil || !strings.Contains(err.Error(), "log in") {
		t.Errorf("error = %v, want a login error", err)
	}
}

// The whole pipeline over a real IMAP connection: mailbox -> Sync -> pending.
func TestSyncOverIMAP(t *testing.T) {
	s := newTestServer(t)
	fillInbox(t, s)
	db, err := store.Open(store.MemoryPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := s.client(time.Time{})

	res, err := inbox.Sync(context.Background(), c, db)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}
	if res.Added != 2 || len(res.Failed) != 0 {
		t.Errorf("first Sync = %+v, want 2 added", res)
	}

	res, err = inbox.Sync(context.Background(), c, db)
	if err != nil {
		t.Fatalf("second Sync() error: %v", err)
	}
	if res.Added != 0 || res.Duplicates != 0 || res.Skipped != 0 {
		t.Errorf("second Sync = %+v, want nothing at all (position was saved)", res)
	}

	s.deliver(t, fixture(t, "transfer_bifast.eml"))
	res, err = inbox.Sync(context.Background(), c, db)
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("third Sync = %+v, want 1 added", res)
	}
	if pending, _ := db.ListPending(); len(pending) != 3 {
		t.Errorf("pending count = %d, want 3", len(pending))
	}
}
