package cli

import (
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

func newSummaryCmd(d Deps) *cobra.Command {
	var monthStr string

	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Show total spending per category for a month",
		Example: `  expense summary
  expense summary --month last
  expense summary --month 2026-10`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			month, err := expense.ParseMonth(monthStr, d.Now())
			if err != nil {
				return err
			}

			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			totals, err := s.Summary(month)
			if err != nil {
				return err
			}
			return printSummary(cmd.OutOrStdout(), month, totals)
		},
	}

	cmd.Flags().StringVar(&monthStr, "month", "current", "month to summarize: YYYY-MM, current or last")
	return cmd
}

// printSummary writes the month title, one row per category, and a grand total.
func printSummary(w io.Writer, month time.Time, totals []expense.CategoryTotal) error {
	title := month.Format("January 2006")
	if len(totals) == 0 {
		_, err := fmt.Fprintf(w, "No expenses in %s.\n", title)
		return err
	}

	var grandTotal int64
	var grandCount int
	for _, t := range totals {
		grandTotal += t.Total
		grandCount += t.Count
	}

	// Right-align the number columns to their widest value (header and total included).
	amountWidth := max(len("AMOUNT"), len(expense.FormatAmount(grandTotal)))
	countWidth := max(len("COUNT"), len(strconv.Itoa(grandCount)))

	if _, err := fmt.Fprintf(w, "%s\n\n", title); err != nil {
		return err
	}

	// tabwriter buffers everything, so a failed write to w is reported by
	// Flush below; the Fprintf results here are deliberately ignored.
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "CATEGORY\t%*s\t%*s\n", amountWidth, "AMOUNT", countWidth, "COUNT")
	for _, t := range totals {
		_, _ = fmt.Fprintf(tw, "%s\t%*s\t%*d\n", t.Category, amountWidth, expense.FormatAmount(t.Total), countWidth, t.Count)
	}
	_, _ = fmt.Fprintf(tw, "TOTAL\t%*s\t%*d\n", amountWidth, expense.FormatAmount(grandTotal), countWidth, grandCount)
	return tw.Flush()
}
