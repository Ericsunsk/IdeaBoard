package composition

import (
	"errors"

	"github.com/spf13/cobra"

	"ideaboard/api/internal/media/reclamation"
)

// newRootCommand builds the process CLI. Subcommand names match the legacy
// hand-dispatched argv exactly (scripts and runbooks invoke
// `finalize-migrate`, `media-reclamation scan`, …). Routing through cobra
// buys uniform `--help`, unknown-flag rejection, and completion for free; an
// unknown command now errors instead of silently booting the server.
func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "api",
		Short: "IdeaBoard API server and maintenance commands",
		RunE: func(*cobra.Command, []string) error {
			runServe()
			return nil
		},
	}
	root.AddCommand(
		newMigrateCommand(),
		newInitAdminCommand(),
		newFinalizeMigrateCommand(),
		newInitCreditWalletsCommand(),
		newMediaReclamationCommand(),
	)
	return root
}

func newMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database schema migrations",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			runMigrate()
			return nil
		},
	}
}

func newInitAdminCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init-admin",
		Short: "Provision the initial admin identity from environment",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			runInitAdmin()
			return nil
		},
	}
}

func newFinalizeMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "finalize-migrate",
		Short: "Finalize database migration (River schema and runtime privileges)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			runFinalizeMigrate()
			return nil
		},
	}
}

func newInitCreditWalletsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init-credit-wallets <verified-identities.json>",
		Short: "Initialize Credit Wallets for existing identities",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			runInitCreditWallets(args)
			return nil
		},
	}
}

func newMediaReclamationCommand() *cobra.Command {
	reclamationCmd := &cobra.Command{
		Use:   "media-reclamation",
		Short: "Media storage consistency inspection",
	}
	scan := &cobra.Command{
		Use:   "scan",
		Short: "Report stale and orphaned media objects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			staleAfter, _ := cmd.Flags().GetDuration("stale-after")
			if staleAfter <= 0 {
				return errors.New("stale-after must be greater than 0 (e.g. --stale-after=30m)")
			}
			runMediaReclamation(staleAfter)
			return nil
		},
	}
	scan.Flags().Duration("stale-after", reclamation.StaleUploadingAfter, "age after which uploading assets are reported as stale")
	reclamationCmd.AddCommand(scan)
	return reclamationCmd
}
