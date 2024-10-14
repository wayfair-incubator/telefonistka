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
		regex             string
		replacement       string
		githubHost        string
		triggeringRepo    string
		triggeringRepoSHA string
		triggeringActor   string
		autoMerge         bool
	)

	var cmd = &cobra.Command{
		Use:   "bump-regex",
		Short: "Bump artifact version in a file using regex",
		Long:  "Bump artifact version in a file using regex.\nThis open a pull request in the target repo.\n",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			params := bumpversion.RegexParams{
				TargetRepo:        targetRepo,
				TargetFile:        targetFile,
				Regex:             regex,
				Replacement:       replacement,
				GithubHost:        githubHost,
				TriggeringRepo:    triggeringRepo,
				TriggeringRepoSHA: triggeringRepoSHA,
				TriggeringActor:   triggeringActor,
				AutoMerge:         autoMerge,
			}
			bumpversion.Regex(params)
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
		&regex,
		"regex-string",
		"r",
		"",
		"Regex used to replace artifact version, e.g. 'tag:\\s*(\\S*)'.",
	)

	cmd.Flags().StringVarP(
		&replacement,
		"replacement-string",
		"n",
		"",
		"Replacement string that includes the version of new artifact, e.g. 'tag: v2.7.1'.",
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
		"auto-merge", false,
		"Automatically merges the created PR, defaults to false.",
	)

	rootCmd.AddCommand(cmd)
}
