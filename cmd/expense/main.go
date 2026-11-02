package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/cli"
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
		Now: time.Now,
	}

	if err := cli.NewRootCmd(deps).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
