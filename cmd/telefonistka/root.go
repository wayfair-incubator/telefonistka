package telefonistka

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "telefonistka",
	Version: "0.0.0",
	Short:   "telefonistka - Safe and Controlled GitOps Promotion Across Environments/Failure-Domains",
	Long: `Telefonistka is a Github webhook server/CLI tool that facilitates change promotion across environments/failure domains in Infrastructure as Code GitOps repos

see https://github.com/wayfair-incubator/telefonistka`,
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Whoops. There was an error while executing your CLI '%s'", err)
		os.Exit(1)
	}
}
