// Package mail reads Bank BRI emails from a mailbox over IMAP (Gmail).
package mail

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/Putz-afk/expense-tracker/internal/inbox"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

const (
	defaultAddr    = "imap.gmail.com:993"
	defaultMailbox = "INBOX"
	defaultFrom    = "bankbri@bri.co.id"

	// Names of the environment variables that hold the credentials. They are
	// never stored in the database or in files that git tracks.
	envUser     = "EXPENSE_IMAP_USER"
	envPassword = "EXPENSE_IMAP_PASSWORD"
)

// Config says where and how to read mail. Empty fields get defaults in New.
type Config struct {
	Addr     string // IMAP server with TLS, default "imap.gmail.com:993"
	Username string
	Password string // for Gmail: an app password, not the account password
	Mailbox  string // default "INBOX"
	From     string // only mail from this address, default BankBRI@bri.co.id

	// TLSConfig customises the TLS connection. Leave nil to trust the system's
	// certificate authorities, which is right for Gmail. Tests set it to trust
	// a throwaway local server.
	TLSConfig *tls.Config

	// Since limits the very first sync (when nothing was read before) to mail
	// received on or after this date, so years of old transactions don't flood
	// the review queue. Zero means no limit.
	Since time.Time
}

// ConfigFromEnv returns a Config with the credentials taken from
// EXPENSE_IMAP_USER and EXPENSE_IMAP_PASSWORD.
func ConfigFromEnv() (Config, error) {
	cfg := Config{Username: os.Getenv(envUser), Password: os.Getenv(envPassword)}
	if cfg.Username == "" || cfg.Password == "" {
		return Config{}, fmt.Errorf("set %s and %s (for Gmail, use an app password)", envUser, envPassword)
	}
	return cfg, nil
}

// Client reads BRI emails. It opens a fresh connection for each call.
type Client struct {
	cfg Config
}

// New returns a Client that implements inbox.Source.
func New(cfg Config) *Client {
	if cfg.Addr == "" {
		cfg.Addr = defaultAddr
	}
	if cfg.Mailbox == "" {
		cfg.Mailbox = defaultMailbox
	}
	if cfg.From == "" {
		cfg.From = defaultFrom
	}
	return &Client{cfg: cfg}
}

// Compile-time check: this line stops the build if *Client ever stops
// satisfying inbox.Mailbox. The blank identifier _ throws the value away.
var _ inbox.Mailbox = (*Client)(nil)

// maxIdle is how long WaitForMail waits before returning anyway. Gmail ends
// idle connections after about half an hour, and a mail that arrives while
// the listener is between two waits would otherwise be noticed late, so this
// also acts as a safety poll.
const maxIdle = 10 * time.Minute

// WaitForMail implements inbox.Waiter. It opens a connection, asks the server
// to push news with IMAP IDLE, and returns when the message count changes,
// after maxIdle, or when ctx is cancelled (then with ctx's error).
func (c *Client) WaitForMail(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// A channel is Go's way to pass a signal between goroutines. The IMAP
	// library calls our handler from its own goroutine whenever the server
	// sends news. The handler must never block it, so the channel has room
	// for one signal and a send that finds it full is simply dropped
	// (select with a default case never waits).
	newMail := make(chan struct{}, 1)
	cl, err := imapclient.DialTLS(c.cfg.Addr, &imapclient.Options{
		TLSConfig: c.cfg.TLSConfig,
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages == nil {
					return // flags changed or similar, not new mail
				}
				select {
				case newMail <- struct{}{}:
				default:
				}
			},
		},
	})
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.cfg.Addr, err)
	}
	defer cl.Close()

	stop := context.AfterFunc(ctx, func() { _ = cl.Close() })
	defer stop()

	if err := cl.Login(c.cfg.Username, c.cfg.Password).Wait(); err != nil {
		return fmt.Errorf("log in as %s: %w", c.cfg.Username, err)
	}
	if _, err := cl.Select(c.cfg.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait(); err != nil {
		return fmt.Errorf("open mailbox %q: %w", c.cfg.Mailbox, err)
	}

	idle, err := cl.Idle()
	if err != nil {
		return fmt.Errorf("start IDLE: %w", wrapCtx(ctx, err))
	}
	defer func() {
		// Leave IDLE politely, then log out. Errors here mean the connection
		// is already gone (cancellation, network), which is fine.
		_ = idle.Close()
		_ = idle.Wait()
		_ = cl.Logout().Wait()
	}()

	timer := time.NewTimer(maxIdle)
	defer timer.Stop()
	select {
	case <-newMail:
		return nil
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FetchSince implements inbox.Source. It reads the mailbox without changing
// it: the mailbox is opened read-only and bodies are fetched with PEEK, so
// nothing is marked as read.
func (c *Client) FetchSince(ctx context.Context, st store.SyncState) (inbox.Batch, error) {
	if err := ctx.Err(); err != nil {
		return inbox.Batch{}, err
	}

	cl, err := imapclient.DialTLS(c.cfg.Addr, &imapclient.Options{TLSConfig: c.cfg.TLSConfig})
	if err != nil {
		return inbox.Batch{}, fmt.Errorf("connect to %s: %w", c.cfg.Addr, err)
	}
	defer cl.Close()

	// imapclient has no context support, so tie it in by hand: if ctx is
	// cancelled, close the connection, which makes any waiting call return.
	// AfterFunc returns a function that cancels this hook; deferring it means
	// the hook is dropped when we finish normally.
	stop := context.AfterFunc(ctx, func() { _ = cl.Close() })
	defer stop()

	if err := cl.Login(c.cfg.Username, c.cfg.Password).Wait(); err != nil {
		return inbox.Batch{}, fmt.Errorf("log in as %s: %w", c.cfg.Username, err)
	}
	defer func() { _ = cl.Logout().Wait() }()

	sel, err := cl.Select(c.cfg.Mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return inbox.Batch{}, fmt.Errorf("open mailbox %q: %w", c.cfg.Mailbox, err)
	}

	// If the server renumbered its messages, the saved position is void.
	lastUID := st.LastUID
	if st.UIDValidity != sel.UIDValidity {
		lastUID = 0
	}

	criteria := &imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "From", Value: c.cfg.From}},
	}
	if lastUID > 0 {
		var uids imap.UIDSet
		uids.AddRange(imap.UID(lastUID+1), 0) // 0 stands for "*", the highest UID
		criteria.UID = []imap.UIDSet{uids}
	} else {
		criteria.Since = c.cfg.Since // zero value = no date limit
	}

	found, err := cl.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return inbox.Batch{}, fmt.Errorf("search mailbox: %w", wrapCtx(ctx, err))
	}

	// IMAP quirk: "n:*" always includes the newest message, even when its UID
	// is below n. So with nothing new we would get the last old message back.
	var uids []imap.UID
	for _, uid := range found.AllUIDs() {
		if uint32(uid) > lastUID {
			uids = append(uids, uid)
		}
	}
	batch := inbox.Batch{UIDValidity: sel.UIDValidity}
	if len(uids) == 0 {
		return batch, nil
	}

	section := &imap.FetchItemBodySection{Peek: true} // the whole message, unread flag untouched
	fetched, err := cl.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return inbox.Batch{}, fmt.Errorf("fetch messages: %w", wrapCtx(ctx, err))
	}

	for _, m := range fetched {
		raw := m.FindBodySection(section)
		// The server matched "From" as a substring; insist on the exact address.
		if raw == nil || !fromMatches(raw, c.cfg.From) {
			continue
		}
		batch.Messages = append(batch.Messages, inbox.Message{UID: uint32(m.UID), Raw: raw})
	}
	// Sync relies on oldest-first order.
	slices.SortFunc(batch.Messages, func(a, b inbox.Message) int {
		return cmp.Compare(a.UID, b.UID)
	})
	return batch, nil
}

// fromMatches reports whether the From header of raw is exactly the address
// want. IMAP's own FROM search matches any part of the header, so
// "bankbri@bri.co.id.evil.example" would pass it too.
func fromMatches(raw []byte, want string) bool {
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		return false
	}
	addr, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil {
		return false
	}
	return strings.EqualFold(addr.Address, want)
}

// wrapCtx reports a cancelled context instead of the confusing "use of closed
// connection" error that closing the connection causes.
func wrapCtx(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(err, ctxErr) {
		return ctxErr
	}
	return err
}
