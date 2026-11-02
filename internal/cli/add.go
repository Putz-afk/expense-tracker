package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/expense"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

func newAddCmd(d Deps) *cobra.Command {
	// Cobra writes the flag values into these variables before RunE is called.
	var amountStr, category, note, dateStr string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an expense",
		Example: `  expense add --amount 25000 --category food --note "lunch"
  expense add --amount 150000 --category transport --date yesterday
  expense add --amount 80000 --category bills --date 2026-10-28`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			amount, err := expense.ParseAmount(amountStr)
			if err != nil {
				return err
			}
			date, err := expense.ParseDate(dateStr, d.Now())
			if err != nil {
				return err
			}

			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			added, err := s.Add(expense.Expense{
				Amount:   amount,
				Category: category,
				Note:     note,
				Date:     date,
			})
			if errors.Is(err, store.ErrUnknownCategory) {
				// %w keeps the original error inside, so errors.Is still works upstream.
				return fmt.Errorf("%w (run `expense categories` to see valid ones)", err)
			}
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added expense #%d: Rp%s %s (%s)\n",
				added.ID, expense.FormatAmount(added.Amount), added.Category,
				added.Date.Format(expense.DateLayout))
			return err
		},
	}

	cmd.Flags().StringVar(&amountStr, "amount", "", "amount in whole rupiah, e.g. 25000 (required)")
	cmd.Flags().StringVar(&category, "category", "", "expense category (required)")
	cmd.Flags().StringVar(&note, "note", "", "optional note")
	cmd.Flags().StringVar(&dateStr, "date", "today", "YYYY-MM-DD, today or yesterday")

	// These only fail if the flag name is misspelled, which is a programming
	// error that the tests would catch immediately.
	_ = cmd.MarkFlagRequired("amount")
	_ = cmd.MarkFlagRequired("category")

	return cmd
}
