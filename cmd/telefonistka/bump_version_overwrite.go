package main

import (
	"github.com/spf13/cobra"
	"github.com/wayfair-incubator/telefonistka/internal/bumpversion"
	"github.com/wayfair-incubator/telefonistka/pkg/utils"
)

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	var (
		targetRepo        string
		targetFile        string
		file              string
		githubHost        string
		triggeringRepo    string
		triggeringRepoSHA string
		triggeringActor   string
		autoMerge         bool
	)

	var cmd = &cobra.Command{
		Use:   "bump-overwrite",
		Short: "Bump artifact version based on provided file content.",
		Long:  "Bump artifact version based on provided file content.\nThis open a pull request in the target repo.",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			params := bumpversion.OverwriteParams{
				TargetRepo:        targetRepo,
				TargetFile:        targetFile,
				File:              file,
				GithubHost:        githubHost,
				TriggeringRepo:    triggeringRepo,
				TriggeringRepoSHA: triggeringRepoSHA,
				TriggeringActor:   triggeringActor,
				AutoMerge:         autoMerge,
			}
			bumpversion.Overwrite(params)
		},
	}

	cmd.Flags().StringVarP(
		&targetRepo,
		"target-repo",
		"t",
		utils.GetEnv("TARGET_REPO", ""),
		"Target Git repository slug(e.g. org-name/repo-name), defaults to TARGET_REPO env var.",
	)

	cmd.Flags().StringVarP(
		&targetFile,
		"target-file",
		"f",
		utils.GetEnv("TARGET_FILE", ""),
		"Target file path(from repo root), defaults to TARGET_FILE env var.",
	)

	cmd.Flags().StringVarP(
		&file,
		"file",
		"c",
		"",
		"File that holds the content the target file will be overwritten with, like \"version.yaml\" or '<(echo -e \"image:\\n  tag: ${VERSION}\")'.",
	)

	cmd.Flags().StringVarP(
		&githubHost,
		"github-host",
		"g",
		"",
		"GitHub instance HOSTNAME, defaults to \"github.com\". This is used for GitHub Enterprise Server instances.",
	)

	cmd.Flags().StringVarP(
		&triggeringRepo,
		"triggering-repo",
		"p",
		utils.GetEnv("GITHUB_REPOSITORY", ""),
		"Github repo triggering the version bump(e.g. `octocat/Hello-World`) defaults to GITHUB_REPOSITORY env var.",
	)

	cmd.Flags().StringVarP(
		&triggeringRepoSHA,
		"triggering-repo-sha",
		"s",
		utils.GetEnv("GITHUB_SHA", ""),
		"Git SHA of triggering repo, defaults to GITHUB_SHA env var.",
	)

	cmd.Flags().StringVarP(
		&triggeringActor,
		"triggering-actor",
		"a",
		utils.GetEnv("GITHUB_ACTOR", ""),
		"GitHub user of the person/bot who triggered the bump, defaults to GITHUB_ACTOR env var.",
	)

	cmd.Flags().BoolVar(
		&autoMerge,
		"auto-merge",
		false,
		"Automatically merges the created PR, defaults to false.",
	)

	rootCmd.AddCommand(cmd)
}
