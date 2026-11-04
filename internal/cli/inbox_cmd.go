package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/expense"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

func newInboxCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Review expenses found in bank emails",
		Long: `inbox lists the expenses that "expense fetch" found in your bank emails and
that still wait for your decision. Accept one to turn it into a real expense,
or reject it to throw it away.`,
		Args: cobra.NoArgs,
		RunE: runInboxList(d),
	}

	// "expense inbox" and "expense inbox list" do the same thing.
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List expenses waiting for review",
		Args:  cobra.NoArgs,
		RunE:  runInboxList(d),
	})
	cmd.AddCommand(newInboxAcceptCmd(d))
	cmd.AddCommand(newInboxRejectCmd(d))
	return cmd
}

// runInboxList returns the function Cobra calls for "inbox" and "inbox list".
// Returning a function that remembers d is called a closure; the same trick
// is used for every command's RunE.
func runInboxList(d Deps) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		s, err := d.OpenStore()
		if err != nil {
			return err
		}
		defer s.Close()

		list, err := s.ListPending()
		if err != nil {
			return err
		}
		return printPending(cmd.OutOrStdout(), list)
	}
}

func newInboxAcceptCmd(d Deps) *cobra.Command {
	var category string

	cmd := &cobra.Command{
		Use:   "accept <id>...",
		Short: "Turn pending items into real expenses",
		Example: `  expense inbox accept 3
  expense inbox accept 3 4 5
  expense inbox accept 6 --category transport`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}

			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			out := cmd.OutOrStdout()
			for _, id := range ids {
				added, err := s.AcceptPending(id, category)
				if errors.Is(err, store.ErrUnknownCategory) {
					return fmt.Errorf("%w (run `expense categories` to see valid ones)", err)
				}
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "Accepted #%d as expense #%d: Rp%s %s (%s)\n",
					id, added.ID, expense.FormatAmount(added.Amount), added.Category,
					added.Date.Format(expense.DateLayout)); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&category, "category", "", "category to use instead of the suggested one")
	return cmd
}

func newInboxRejectCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:     "reject <id>...",
		Short:   "Throw pending items away",
		Example: "  expense inbox reject 3 4",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}

			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			for _, id := range ids {
				if err := s.RejectPending(id); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Rejected #%d.\n", id); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// parseIDs converts every argument to a positive ID before anything is
// changed, so "accept 1 abc" fails without having accepted #1.
func parseIDs(args []string) ([]int64, error) {
	ids := make([]int64, 0, len(args))
	for _, a := range args {
		id, err := strconv.ParseInt(a, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid pending expense ID %q: must be a positive whole number", a)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// printPending writes the review queue as an aligned table.
func printPending(w io.Writer, list []expense.Pending) error {
	if len(list) == 0 {
		_, err := fmt.Fprintln(w, "No pending expenses.")
		return err
	}

	amountWidth := len("AMOUNT")
	amounts := make([]string, len(list))
	var total int64
	for i, p := range list {
		amounts[i] = expense.FormatAmount(p.Amount)
		if len(amounts[i]) > amountWidth {
			amountWidth = len(amounts[i])
		}
		total += p.Amount
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "ID\tDATE\tTYPE\tSUGGESTED\t%*s\tPAYEE\n", amountWidth, "AMOUNT")
	for i, p := range list {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%*s\t%s\n",
			p.ID, p.Date.Format(expense.DateLayout), p.Kind, p.Category, amountWidth, amounts[i], p.Payee)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	_, err := fmt.Fprintf(w, "\n%d waiting, total Rp%s\n"+
		"Accept with `expense inbox accept <id>` (add --category to change it), reject with `expense inbox reject <id>`.\n",
		len(list), expense.FormatAmount(total))
	return err
}
