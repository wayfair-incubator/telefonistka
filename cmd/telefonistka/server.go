package main

import (
	"github.com/spf13/cobra"
	"github.com/wayfair-incubator/telefonistka/internal/server"
)

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	var cmd = &cobra.Command{
		Use:   "server",
		Short: "Runs the web server that listens to GitHub webhooks",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			server.Serve()
		},
	}

	rootCmd.AddCommand(cmd)
}
