package inbox

import (
	"context"
	"fmt"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/store"
)

// Waiter blocks until the mailbox might have new mail. It may also return
// early or on a timer (a false alarm only costs one cheap Sync). It must
// return promptly when ctx is cancelled.
type Waiter interface {
	WaitForMail(ctx context.Context) error
}

// Mailbox is a Source that can also announce new mail.
type Mailbox interface {
	Source
	Waiter
}

// Default delays between retries after a failure.
const (
	defaultMinBackoff = 5 * time.Second
	defaultMaxBackoff = 5 * time.Minute
)

// Listener keeps the review queue up to date: it syncs, waits for new mail,
// syncs again, and so on until its context is cancelled.
type Listener struct {
	Source Source
	Waiter Waiter
	Store  *store.Store

	// Report is called after every sync with its result and error, and when
	// waiting for mail fails (then with an empty Result). The Listener keeps
	// running after errors, so this is the only place a user sees them.
	Report func(Result, error)

	// MinBackoff and MaxBackoff bound the wait before retrying after an
	// error. The wait doubles on every consecutive failure. Zero values
	// mean 5 seconds and 5 minutes.
	MinBackoff, MaxBackoff time.Duration

	// sleep waits for d or until ctx is done, and says whether the full time
	// passed. Tests replace it to avoid really waiting.
	sleep func(ctx context.Context, d time.Duration) bool
}

// Run blocks until ctx is cancelled, then returns nil (stopping because the
// user pressed Ctrl+C is not an error). Network and mailbox errors never end
// it: they are reported and retried with growing delays, so a Wi-Fi drop
// overnight does not kill the listener.
func (l *Listener) Run(ctx context.Context) error {
	minBackoff, maxBackoff := l.MinBackoff, l.MaxBackoff
	if minBackoff <= 0 {
		minBackoff = defaultMinBackoff
	}
	if maxBackoff < minBackoff {
		maxBackoff = max(defaultMaxBackoff, minBackoff)
	}
	sleep := l.sleep
	if sleep == nil {
		sleep = sleepContext
	}

	backoff := minBackoff
	for {
		res, err := Sync(ctx, l.Source, l.Store)
		if err != nil && ctx.Err() != nil {
			return nil // interrupted mid-sync
		}
		l.Report(res, err)

		if err == nil {
			err = l.Waiter.WaitForMail(ctx)
			if ctx.Err() != nil {
				return nil
			}
			if err != nil {
				l.Report(Result{}, fmt.Errorf("wait for mail: %w", err))
			}
		}

		if err == nil {
			backoff = minBackoff // healthy again
			continue
		}
		if !sleep(ctx, backoff) {
			return nil
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// sleepContext waits for d, or until ctx is cancelled, whichever is first.
// It reports whether the whole time passed.
//
// select waits on several channels at once and runs whichever case is ready
// first. ctx.Done() is a channel that is closed when ctx is cancelled, which
// is how cancellation reaches code that is sleeping.
func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
