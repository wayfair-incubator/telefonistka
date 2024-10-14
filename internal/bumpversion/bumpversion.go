package bumpversion

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	log "github.com/sirupsen/logrus"

	"github.com/wayfair-incubator/telefonistka/internal/github"
	"github.com/wayfair-incubator/telefonistka/pkg/githubapi"
	"github.com/wayfair-incubator/telefonistka/pkg/utils"
)

type OverwriteParams struct {
	TargetRepo        string
	TargetFile        string
	File              string
	GithubHost        string
	TriggeringRepo    string
	TriggeringRepoSHA string
	TriggeringActor   string
	AutoMerge         bool
}

func Overwrite(params OverwriteParams) {
	b, err := os.ReadFile(params.File)
	if err != nil {
		log.Fatalf("Failed to read file %s, %v", params.File, err)
	}
	newFileContent := string(b)

	if params.GithubHost != "" {
		githubRestAltURL := "https://" + params.GithubHost + "/api/v3"
		log.Infof("Github REST API endpoint is configured to %s", githubRestAltURL)
	}

	ctx := context.Background()
	ghPrClientDetails := githubapi.GhPrClientDetails{
		GhClientPair: github.NewGhClientPair(ctx, params.TargetRepo),
		Ctx:          ctx,
		Owner:        strings.Split(params.TargetRepo, "/")[0],
		Repo:         strings.Split(params.TargetRepo, "/")[1],
		PrLogger:     log.WithFields(log.Fields{}), // TODO what fields should be here?
	}

	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()

	initialFileContent, statusCode, err := githubapi.GetFileContent(ghPrClientDetails, defaultBranch, params.TargetFile)
	if statusCode == 404 {
		ghPrClientDetails.PrLogger.Infof("File %s was not found\n", params.TargetFile)
	} else if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to fetch file content:%s\n", err)
		os.Exit(1)
	}

	edits := myers.ComputeEdits(span.URIFromPath(""), initialFileContent, newFileContent)
	ghPrClientDetails.PrLogger.Infof("Diff:\n%s", gotextdiff.ToUnified("Before", "After", initialFileContent, edits))

	if err = githubapi.BumpVersion(
		ghPrClientDetails,
		"main",
		params.TargetFile,
		newFileContent,
		params.TriggeringRepo,
		params.TriggeringRepoSHA,
		params.TriggeringActor,
		params.AutoMerge,
	); err != nil {
		log.Fatalf("Failed to bump version: %v", err)
	}
}

type RegexParams struct {
	TargetRepo        string
	TargetFile        string
	Regex             string
	Replacement       string
	GithubHost        string
	TriggeringRepo    string
	TriggeringRepoSHA string
	TriggeringActor   string
	AutoMerge         bool
}

func Regex(params RegexParams) {
	if params.GithubHost != "" {
		githubAltUrl := "https://" + params.GithubHost + "/api/v3"
		log.Infof("Github REST API endpoint is configured to %s", githubAltUrl)
	}

	ctx := context.Background()
	ghPrClientDetails := githubapi.GhPrClientDetails{
		GhClientPair: github.NewGhClientPair(ctx, params.TargetRepo),
		Ctx:          ctx,
		Owner:        strings.Split(params.TargetRepo, "/")[0],
		Repo:         strings.Split(params.TargetRepo, "/")[1],
		PrLogger:     log.WithFields(log.Fields{}), // TODO what fields should be here?
	}

	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()

	initialFileContent, _, err := githubapi.GetFileContent(ghPrClientDetails, defaultBranch, params.TargetFile)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to fetch file content:%s\n", err)
		os.Exit(1)
	}

	r := regexp.MustCompile(params.Regex)
	newFileContent := r.ReplaceAllString(initialFileContent, params.Replacement)
	edits := myers.ComputeEdits(span.URIFromPath(""), initialFileContent, newFileContent)

	ghPrClientDetails.PrLogger.Infof("Diff:\n%s", gotextdiff.ToUnified("Before", "After", initialFileContent, edits))

	if err = githubapi.BumpVersion(
		ghPrClientDetails,
		"main",
		params.TargetFile,
		newFileContent,
		params.TriggeringRepo,
		params.TriggeringRepoSHA,
		params.TriggeringActor,
		params.AutoMerge,
	); err != nil {
		log.Fatalf("Failed to bump version: %v", err)
	}
}

type YamlParams struct {
	TargetRepo        string
	TargetFile        string
	Address           string
	Value             string
	GithubHost        string
	TriggeringRepo    string
	TriggeringRepoSHA string
	TriggeringActor   string
	AutoMerge         bool
}

func Yaml(params YamlParams) {
	if params.GithubHost != "" {
		githubAltUrl := "https://" + params.GithubHost + "/api/v3"
		log.Infof("Github REST API endpoint is configured to %s", githubAltUrl)
	}

	ctx := context.Background()
	ghPrClientDetails := githubapi.GhPrClientDetails{
		GhClientPair: github.NewGhClientPair(ctx, params.TargetRepo),
		Ctx:          ctx,
		Owner:        strings.Split(params.TargetRepo, "/")[0],
		Repo:         strings.Split(params.TargetRepo, "/")[1],
		PrLogger:     log.WithFields(log.Fields{}), // TODO what fields should be here?
	}

	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	initialFileContent, _, err := githubapi.GetFileContent(ghPrClientDetails, defaultBranch, params.TargetFile)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to fetch file content:%s\n", err)
		os.Exit(1)
	}
	newFileContent, err := utils.UpdateYaml(initialFileContent, params.Address, params.Value)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to update yaml:%s\n", err)
		os.Exit(1)
	}
	edits := myers.ComputeEdits(span.URIFromPath(""), initialFileContent, newFileContent)

	ghPrClientDetails.PrLogger.Infof("Diff:\n%s", gotextdiff.ToUnified("Before", "After", initialFileContent, edits))

	if err := githubapi.BumpVersion(
		ghPrClientDetails,
		"main",
		params.TargetFile,
		newFileContent,
		params.TriggeringRepo,
		params.TriggeringRepoSHA,
		params.TriggeringActor,
		params.AutoMerge,
	); err != nil {
		log.Fatalf("Failed to bump version: %v", err)
	}
}
