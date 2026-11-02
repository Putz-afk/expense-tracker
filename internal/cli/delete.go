package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

func newDeleteCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete an expense (asks for confirmation)",
		Example: "  expense delete 3",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("invalid expense ID %q: must be a positive whole number", args[0])
			}

			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			// Look it up first so the prompt can show exactly what will be deleted.
			e, err := s.Get(id)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Delete #%d (Rp%s %s, %s)? [y/N]: ",
				e.ID, expense.FormatAmount(e.Amount), e.Category, e.Date.Format(expense.DateLayout)); err != nil {
				return err
			}

			// cmd.InOrStdin() is os.Stdin normally, and a fake reader in tests.
			confirmed, err := readYes(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read answer: %w", err)
			}
			if !confirmed {
				_, err = fmt.Fprintln(out, "Cancelled.")
				return err
			}

			if err := s.Delete(id); err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "Deleted expense #%d.\n", id)
			return err
		},
	}
}

// readYes reads one line and reports whether it is "y" or "yes" (any case).
// Everything else, including an empty line or end of input, means no.
func readYes(in io.Reader) (bool, error) {
	line, err := bufio.NewReader(in).ReadString('\n')
	// At end of input ReadString returns io.EOF along with any text before it,
	// so "y" with no trailing newline still counts as an answer.
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}
