// Command migrationhistory exposes the shared history policy to shell gates.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"ideaboard/api/internal/db/migrationhistory"
)

func main() {
	var options migrationhistory.Options
	var listChanged bool
	flag.StringVar(&options.RepositoryRoot, "repository", ".", "Git repository directory")
	flag.StringVar(&options.MigrationsDir, "migrations", "internal/db/migrations", "migration directory")
	flag.StringVar(&options.BaseSHA, "base", "", "base commit (optional for working-tree checks)")
	flag.StringVar(&options.HeadSHA, "head", "HEAD", "head commit")
	flag.BoolVar(&listChanged, "list-changed", false, "list current changed SQL paths for linting without enforcing history policy")
	flag.Parse()
	history, err := migrationhistory.Inspect(context.Background(), options)
	if err == nil {
		if listChanged {
			for _, path := range history.ChangedFiles {
				fmt.Println(path)
			}
		} else {
			err = history.Validate()
			if err == nil && history.CleanBreak {
				fmt.Println("clean-break rebaseline detected")
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
