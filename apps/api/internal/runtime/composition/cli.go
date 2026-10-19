package composition

import (
	"github.com/spf13/cobra"

	"ideaboard/api/internal/platform/config"
)

// newRootCommand builds the process CLI. Subcommand names match the legacy
// hand-dispatched argv exactly (scripts and runbooks invoke
// `finalize-migrate`, `media-reclamation scan`, …). Routing through cobra
// buys uniform `--help`, unknown-flag rejection, and completion for free; an
// unknown command now errors instead of silently booting the server.
//
// `--mode` overrides API_MODE for the serve path only (ADR-0021); every
// other subcommand ignores it.
func newRootCommand() *cobra.Command {
	var modeOverride string
	root := &cobra.Command{
		Use:   "api",
		Short: "IdeaBoard API server and maintenance commands",
		RunE: func(*cobra.Command, []string) error {
			runServe(resolveServeMode(modeOverride))
			return nil
		},
	}
	root.PersistentFlags().StringVar(&modeOverride, "mode", "", "process role: all|http-only|worker-only (overrides API_MODE for serve)")
	root.AddCommand(
		newMigrateCommand(),
		newInitAdminCommand(),
		newFinalizeMigrateCommand(),
		newInitCreditWalletsCommand(),
		newBackfillBoardPreviewsCommand(),
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

// resolveServeMode applies the explicit --mode flag over the API_MODE env
// value already loaded into cfg. An empty flag keeps the env value; an
// unknown flag value fails fast (explicit user input never falls back).
func resolveServeMode(flag string) func(*config.Config) {
	return func(cfg *config.Config) {
		if cfg == nil {
			return
		}
		if flag == "" {
			return
		}
		mode, valid := config.ParseProcessMode(flag)
		if !valid {
			fatal("invalid --mode (want all|http-only|worker-only)", "value", flag)
		}
		cfg.Mode = mode
	}
}
