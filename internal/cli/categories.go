package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newCategoriesCmd builds "expense categories" (list) with an "add" subcommand.
// Running the parent command by itself lists; that's why it has its own RunE.
func newCategoriesCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "categories",
		Short: "List categories, or add a new one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			names, err := s.ListCategories()
			if err != nil {
				return err
			}
			for _, name := range names {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), name); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.AddCommand(newCategoriesAddCmd(d))
	return cmd
}

func newCategoriesAddCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new category",
		Example: `  expense categories add groceries
  expense categories add eating out`,
		// Accept several words so "eating out" works without quotes.
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := d.OpenStore()
			if err != nil {
				return err
			}
			defer s.Close()

			name, err := s.AddCategory(strings.Join(args, " "))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added category %q\n", name)
			return err
		},
	}
}
