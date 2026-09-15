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
		newPreflightCommand(),
		newRepairCommand(),
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

func newPreflightCommand() *cobra.Command {
	preflight := &cobra.Command{
		Use:   "preflight",
		Short: "Read-only startup safety checks",
	}
	preflight.AddCommand(&cobra.Command{
		Use:   "task-leases",
		Short: "Report running tasks missing a lease token",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			runPreflight()
			return nil
		},
	})
	return preflight
}

func newRepairCommand() *cobra.Command {
	repair := &cobra.Command{
		Use:   "repair",
		Short: "Bounded repair operations (safe by default)",
	}
	taskLeases := &cobra.Command{
		Use:   "task-leases",
		Short: "Reset running tasks missing a lease token back to pending",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			to, _ := cmd.Flags().GetString("to")
			yes, _ := cmd.Flags().GetBool("yes")
			limit, _ := cmd.Flags().GetInt("limit")
			opts := resolveRepairTaskLeasesOptions(dryRun, cmd.Flags().Changed("dry-run"), to, yes, limit)
			runRepairTaskLeases(opts)
			return nil
		},
	}
	taskLeases.Flags().Bool("dry-run", true, "preview only; no mutation")
	taskLeases.Flags().String("to", "", "target status for mutation (supported: pending)")
	taskLeases.Flags().Bool("yes", false, "confirm mutation")
	taskLeases.Flags().Int("limit", defaultRepairLimit, "max rows to update")
	repair.AddCommand(taskLeases)
	return repair
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
