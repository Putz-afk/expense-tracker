package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/cli"
	"github.com/Putz-afk/expense-tracker/internal/inbox"
	"github.com/Putz-afk/expense-tracker/internal/mail"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

func main() {
	deps := cli.Deps{
		OpenStore: func() (*store.Store, error) {
			path, err := store.DefaultPath()
			if err != nil {
				return nil, err
			}
			return store.Open(path)
		},
		OpenSource: func(since time.Time) (inbox.Mailbox, error) {
			cfg, err := mail.ConfigFromEnv()
			if err != nil {
				return nil, err
			}
			cfg.Since = since
			return mail.New(cfg), nil
		},
		Now: time.Now,
	}

	// The context is cancelled when the user presses Ctrl+C. Commands that
	// take a while (fetch, listen) watch it and stop cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)

	err := cli.NewRootCmd(deps).ExecuteContext(ctx)
	stop() // os.Exit below skips deferred calls, so release the signal handler explicitly
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
