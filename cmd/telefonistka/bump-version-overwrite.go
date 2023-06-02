package telefonistka

import (
	"context"
	"os"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/githubapi"
)

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	var targetRepo string
	var targetFile string
	var newFileContent string
	var githubHost string
	var triggeringRepo string
	var triggeringRepoSHA string
	var triggeringActor string
	eventCmd := &cobra.Command{
		Use:   "bump-overwrite",
		Short: "Bump artifact version in a file using regex",
		Long:  "Bump artifact version in a file using regex.\nThis open a pull request in the target repo.",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			bumpVersionOverwrite(targetRepo, targetFile, newFileContent, githubHost, triggeringRepo, triggeringRepoSHA, triggeringActor)
		},
	}
	eventCmd.Flags().StringVarP(&targetRepo, "target-repo", "t", getEnv("TARGET_REPO", ""), "Target Git repository slug(e.g. org-name/repo-name), defaults to TARGET_REPO env var")
	eventCmd.Flags().StringVarP(&targetFile, "target-file", "f", getEnv("TARGET_FILE", ""), "Target file path(from repo root), defaults to TARGET_FILE env var")
	eventCmd.Flags().StringVarP(&newFileContent, "file-content", "c", "", "New file content(e.g. 'image:\\n  tag: v3.4.9')  ")
	eventCmd.Flags().StringVarP(&githubHost, "github-host", "g", "", "GitHub instance HOSTNAME, defaults to \"github.com\". This is used for GitHub Enterprise Server instances")
	eventCmd.Flags().StringVarP(&triggeringRepo, "triggering-repo", "p", getEnv("GITHUB_REPOSITORY", ""), "Github repo triggering the version bump(e.g. `octocat/Hello-World`) defaults to GITHUB_REPOSITORY env var")
	eventCmd.Flags().StringVarP(&triggeringRepoSHA, "triggering-repo-sha", "s", getEnv("GITHUB_SHA", ""), "Git SHA of triggering repo, defaults to GITHUB_SHA env var.")
	eventCmd.Flags().StringVarP(&triggeringActor, "triggering-actor", "a", getEnv("GITHUB_ACTOR", ""), "GitHub user of the person/bot who triggered the bump, defaults to GITHUB_ACTOR env var.")
	rootCmd.AddCommand(eventCmd)
}

func bumpVersionOverwrite(targetRepo string, targetFile string, newFileContent string, githubHost string, triggeringRepo string, triggeringRepoSHA string, triggeringActor string) {
	ctx := context.Background()
	var githubRestAltURL string

	if githubHost != "" {
		githubRestAltURL = "https://" + githubHost + "/api/v3"
		log.Infof("Github REST API endpoint is configured to %s", githubRestAltURL)
	}

	var ghPrClientDetails githubapi.GhPrClientDetails

	ghPrClientDetails.Ghclient = githubapi.CreateGithubRestClient(getCrucialEnv("GITHUB_OAUTH_TOKEN"), githubRestAltURL, ctx)
	ghPrClientDetails.Ctx = ctx
	ghPrClientDetails.Owner = strings.Split(targetRepo, "/")[0]
	ghPrClientDetails.Repo = strings.Split(targetRepo, "/")[1]
	ghPrClientDetails.PrLogger = log.WithFields(log.Fields{}) // TODO what fields should be here?

	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	initialFileContent, err, statusCode := githubapi.GetFileContent(ghPrClientDetails, defaultBranch, targetFile)
	if statusCode == 404 {
		ghPrClientDetails.PrLogger.Infof("File %s was not found\n", targetFile)
	} else if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to fetch file content:%s\n", err)
		os.Exit(1)
	}

	edits := myers.ComputeEdits(span.URIFromPath(""), initialFileContent, newFileContent)
	ghPrClientDetails.PrLogger.Infof("Diff:\n%s", gotextdiff.ToUnified("Before", "After", initialFileContent, edits))

	err = githubapi.BumpVersion(ghPrClientDetails, "main", targetFile, newFileContent, triggeringRepo, triggeringRepoSHA, triggeringActor)
	if err != nil {
		log.Errorf("Failed to bump version: %v", err)
		os.Exit(1)
	}
}
