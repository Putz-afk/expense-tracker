package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/expense"
	"github.com/Putz-afk/expense-tracker/internal/inbox"
)

func newFetchCmd(d Deps) *cobra.Command {
	var sinceStr string

	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Read new Bank BRI emails once and queue them for review",
		Long: `fetch connects to your mailbox, reads the Bank BRI transaction emails that
arrived since the last run, and adds them to the review queue. Nothing becomes
an expense until you accept it with "expense inbox accept".

Set EXPENSE_IMAP_USER and EXPENSE_IMAP_PASSWORD first. For Gmail the password
must be an app password (Google Account > Security > App passwords).

--since only matters the very first time: it keeps old emails from flooding the
queue. After that, fetch continues where it stopped.`,
		Example: `  expense fetch
  expense fetch --since 2026-10-01`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			since, err := parseSince(sinceStr, d.Now())
			if err != nil {
				return err
			}

			src, err := d.OpenSource(since)
			if err != nil {
				return err
			}
			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			// cmd.Context() carries Ctrl+C (see main), so a slow fetch can be stopped.
			res, err := inbox.Sync(cmd.Context(), src, s)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if err := printSyncResult(out, cmd.ErrOrStderr(), res); err != nil {
				return err
			}
			pending, err := s.ListPending()
			if err != nil {
				return err
			}
			return printReviewHint(out, len(pending))
		},
	}

	cmd.Flags().StringVar(&sinceStr, "since", "", "first sync only: ignore emails before this date (YYYY-MM-DD; default: start of this month)")
	return cmd
}

// parseSince turns the --since flag into a date. Empty means the first day of
// the current month.
func parseSince(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return expense.ParseMonth("current", now)
	}
	return expense.ParseDate(s, now)
}

// printSyncResult describes one Sync on out, and any emails that could not be
// read on errOut (so they don't get lost in piped output).
func printSyncResult(out, errOut io.Writer, res inbox.Result) error {
	if _, err := fmt.Fprintf(out, "%d new, %d already saved, %d skipped (not transactions)\n",
		res.Added, res.Duplicates, res.Skipped); err != nil {
		return err
	}
	for _, f := range res.Failed {
		if _, err := fmt.Fprintf(errOut, "warning: could not read message %d: %v\n", f.UID, f.Err); err != nil {
			return err
		}
	}
	return nil
}

func printReviewHint(w io.Writer, pending int) error {
	if pending == 0 {
		_, err := fmt.Fprintln(w, "Nothing waiting for review.")
		return err
	}
	_, err := fmt.Fprintf(w, "%d waiting for review. Run `expense inbox` to see them.\n", pending)
	return err
}
