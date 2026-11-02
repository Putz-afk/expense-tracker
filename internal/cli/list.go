package cli

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/expense"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

func newListCmd(d Deps) *cobra.Command {
	var monthStr, category string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List expenses, newest first",
		Example: `  expense list
  expense list --month 2026-11
  expense list --month last --category food`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var filter expense.Filter
			if monthStr != "" {
				month, err := expense.ParseMonth(monthStr, d.Now())
				if err != nil {
					return err
				}
				filter.Month = month
			}
			filter.Category = category

			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			list, err := s.List(filter)
			if errors.Is(err, store.ErrUnknownCategory) {
				return fmt.Errorf("%w (run `expense categories` to see valid ones)", err)
			}
			if err != nil {
				return err
			}

			return printExpenses(cmd.OutOrStdout(), list)
		},
	}

	cmd.Flags().StringVar(&monthStr, "month", "", "only this month: YYYY-MM, current or last")
	cmd.Flags().StringVar(&category, "category", "", "only this category")
	return cmd
}

// printExpenses writes an aligned table followed by a count and total line.
// io.Writer is Go's "anything you can write bytes to" interface: os.Stdout,
// a file, or a bytes.Buffer in tests.
func printExpenses(w io.Writer, list []expense.Expense) error {
	if len(list) == 0 {
		_, err := fmt.Fprintln(w, "No expenses found.")
		return err
	}

	// tabwriter aligns columns separated by tabs but can only pad on the right,
	// so right-align the amount column ourselves to the widest value.
	amountWidth := len("AMOUNT")
	amounts := make([]string, len(list))
	var total int64
	for i, e := range list {
		amounts[i] = expense.FormatAmount(e.Amount)
		if len(amounts[i]) > amountWidth {
			amountWidth = len(amounts[i])
		}
		total += e.Amount
	}

	// tabwriter buffers everything, so a failed write to w is reported by
	// Flush below; the Fprintf results here are deliberately ignored.
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "ID\tDATE\tCATEGORY\t%*s\tNOTE\n", amountWidth, "AMOUNT")
	for i, e := range list {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%*s\t%s\n",
			e.ID, e.Date.Format(expense.DateLayout), e.Category, amountWidth, amounts[i], e.Note)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	noun := "expenses"
	if len(list) == 1 {
		noun = "expense"
	}
	_, err := fmt.Fprintf(w, "\n%d %s, total Rp%s\n", len(list), noun, expense.FormatAmount(total))
	return err
}
