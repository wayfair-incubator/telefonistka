package main

import (
	"github.com/spf13/cobra"
	"github.com/wayfair-incubator/telefonistka/internal/github"
	"github.com/wayfair-incubator/telefonistka/pkg/utils"
)

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	var (
		kind     string
		filePath string
	)

	var cmd = &cobra.Command{
		Use:   "event",
		Short: "Handles a GitHub event based on event JSON file",
		Long:  "Handles a GitHub event based on event JSON file.\nThis operation mode was was built with GitHub Actions in mind",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			github.Event(kind, filePath)
		},
	}

	cmd.Flags().StringVarP(
		&kind,
		"type",
		"t",
		utils.GetEnv("GITHUB_EVENT_NAME", ""),
		"Event type, defaults to GITHUB_EVENT_NAME env var",
	)

	cmd.Flags().StringVarP(
		&filePath,
		"file",
		"f",
		utils.GetEnv("GITHUB_EVENT_PATH", ""),
		"File path for event JSON, defaults to GITHUB_EVENT_PATH env var",
	)

	rootCmd.AddCommand(cmd)
}
