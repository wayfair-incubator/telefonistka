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
		address           string
		replacement       string
		githubHost        string
		triggeringRepo    string
		triggeringRepoSHA string
		triggeringActor   string
		autoMerge         bool
	)

	var cmd = &cobra.Command{
		Use:   "bump-yaml",
		Short: "Bump artifact version in a file using yaml selector",
		Long: `Bump artifact version in a file using yaml selector.
	This will open a pull request in the target repo.
	This command uses yq selector to find the yaml value to replace.
	`,
		Args: cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			params := bumpversion.YamlParams{
				TargetRepo:        targetRepo,
				TargetFile:        targetFile,
				Address:           address,
				Value:             replacement,
				GithubHost:        githubHost,
				TriggeringRepo:    triggeringRepo,
				TriggeringRepoSHA: triggeringRepoSHA,
				TriggeringActor:   triggeringActor,
				AutoMerge:         autoMerge,
			}
			bumpversion.Yaml(params)
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

	cmd.Flags().StringVar(
		&address,
		"address",
		"",
		"Yaml value address described as a yq selector, e.g. '.db.[] | select(.name == \"postgres\").image.tag'.",
	)

	cmd.Flags().StringVarP(
		&replacement,
		"replacement-string",
		"n",
		"",
		"Replacement string that includes the version value of new artifact, e.g. 'v2.7.1'.",
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
