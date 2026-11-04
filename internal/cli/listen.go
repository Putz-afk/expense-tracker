package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/inbox"
)

func newListenCmd(d Deps) *cobra.Command {
	var sinceStr string

	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Keep running and queue new Bank BRI emails as they arrive",
		Long: `listen does what "expense fetch" does, then stays connected and checks again
whenever your mailbox gets new mail. Leave it running in a terminal and press
Ctrl+C to stop. New transactions appear in "expense inbox".

If the network drops, listen reports the error and tries again with growing
pauses (5 seconds up to 5 minutes), so you don't need to restart it.

Set EXPENSE_IMAP_USER and EXPENSE_IMAP_PASSWORD first; see "expense fetch --help".`,
		Example: "  expense listen",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			since, err := parseSince(sinceStr, d.Now())
			if err != nil {
				return err
			}
			mb, err := d.OpenSource(since)
			if err != nil {
				return err
			}
			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
			if _, err := fmt.Fprintln(out, "Listening for Bank BRI emails. Press Ctrl+C to stop."); err != nil {
				return err
			}

			// Report cannot return an error to anyone, so failed writes to the
			// terminal are ignored here (there is nowhere left to report them).
			announcedFirst := false
			l := &inbox.Listener{
				Source: mb,
				Waiter: mb,
				Store:  s,
				Report: func(res inbox.Result, err error) {
					stamp := d.Now().Format(time.TimeOnly)
					if err != nil {
						_, _ = fmt.Fprintf(errOut, "[%s] error: %v (will retry)\n", stamp, err)
						return
					}
					// Stay quiet when nothing happened, except to confirm the
					// first successful check.
					if res.Added == 0 && len(res.Failed) == 0 && announcedFirst {
						return
					}
					announcedFirst = true
					_, _ = fmt.Fprintf(out, "[%s] ", stamp)
					_ = printSyncResult(out, errOut, res)
					if res.Added > 0 {
						_, _ = fmt.Fprintln(out, "         Run `expense inbox` to review.")
					}
				},
			}

			// cmd.Context() is cancelled by Ctrl+C (see main), which ends Run.
			if err := l.Run(cmd.Context()); err != nil {
				return err
			}
			_, err = fmt.Fprintln(out, "Stopped.")
			return err
		},
	}

	cmd.Flags().StringVar(&sinceStr, "since", "", "first sync only: ignore emails before this date (YYYY-MM-DD; default: start of this month)")
	return cmd
}
