// Package cli defines the Cobra commands for the expense tracker.
package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/inbox"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

// Deps holds the outside things commands need. main passes the real ones;
// tests pass fakes (a temporary database, a fixed clock).
//
// Commands call OpenStore only when they actually run, so "expense --help"
// never touches the database file.
type Deps struct {
	OpenStore func() (*store.Store, error)
	// OpenSource returns the mailbox to read bank emails from. since limits a
	// first-ever sync to mail received on or after that date.
	OpenSource func(since time.Time) (inbox.Mailbox, error)
	Now        func() time.Time
}

// NewRootCmd builds the top-level "expense" command and attaches the
// subcommands to it.
//
// Returning a fresh command from a function (instead of using a package-level
// variable) makes it easy to test: each test gets its own clean instance.
func NewRootCmd(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "expense",
		Short: "A small CLI expense tracker",
		Long:  "expense records your spending in a local SQLite database and summarizes it by category.",
		// Don't print usage text on runtime errors; only on wrong usage.
		SilenceUsage: true,
		// main prints errors itself, so Cobra shouldn't print them a second time.
		SilenceErrors: true,
	}

	root.AddCommand(newAddCmd(d))
	root.AddCommand(newListCmd(d))
	root.AddCommand(newDeleteCmd(d))
	root.AddCommand(newSummaryCmd(d))
	root.AddCommand(newCategoriesCmd(d))
	root.AddCommand(newFetchCmd(d))
	root.AddCommand(newInboxCmd(d))
	root.AddCommand(newListenCmd(d))
	return root
}
