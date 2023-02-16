package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/google/go-github/v48/github"
	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
	cfg "github.com/wayfair-incubator/telefonistka/internal/pkg/configuration"
	githubapi "github.com/wayfair-incubator/telefonistka/internal/pkg/githubapi"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/promotion"
)

func commentPlanInPR(ghPrClientDetails githubapi.GhPrClientDetails, promotions map[string]promotion.PromotionInstance) {
	var templateOutput bytes.Buffer
	dryRunMsgTemplate, err := template.New("dryRunMsg").ParseFiles("templates/dry-run-pr-comment.gotmpl")
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to parse template: err=%v", err)
	}
	err = dryRunMsgTemplate.ExecuteTemplate(&templateOutput, "dryRunMsg", promotions)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to execute template: err=%v", err)
	}
	// templateOutputString := templateOutput.String()
	err = ghPrClientDetails.CommentOnPr(templateOutput.String())
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to comment plan in PR: err=%v", err)
	}
}

func HandleWebhook(mainGithubClient *github.Client, prApproverGithubClient *github.Client, githubGraphQlClient *githubv4.Client, ctx context.Context, githubWebhookSecret []byte) func(http.ResponseWriter, *http.Request) {
	if mainGithubClient == nil {
		panic("nil GH session!")
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// payload, err := ioutil.ReadAll(r.Body)
		payload, err := github.ValidatePayload(r, githubWebhookSecret)
		if err != nil {
			log.Errorf("error reading request body: err=%s\n", err)
			prom.InstrumentWebhookHit("validation_failed")
			return
		}

		eventPayloadInterface, err := github.ParseWebHook(github.WebHookType(r), payload)
		if err != nil {
			log.Errorf("could not parse webhook: err=%s\n", err)
			prom.InstrumentWebhookHit("parsing_failed")
			return
		}
		prom.InstrumentWebhookHit("successful")

		switch eventPayload := eventPayloadInterface.(type) {
		case *github.PushEvent:
			// this is a commit push, do something with it?
			log.Infoln("is PushEvent")
		case *github.PullRequestEvent:
			// this is a pull request, do something with it
			log.Infoln("is PullRequestEvent")

			prLogger := log.WithFields(log.Fields{
				"repo":     *eventPayload.Repo.Owner.Login + "/" + *eventPayload.Repo.Name,
				"prNumber": *eventPayload.PullRequest.Number,
			})

			ghPrClientDetails := githubapi.GhPrClientDetails{
				Ctx:      ctx,
				Ghclient: mainGithubClient,
				Labels:   eventPayload.PullRequest.Labels,
				Owner:    *eventPayload.Repo.Owner.Login,
				Repo:     *eventPayload.Repo.Name,
				PrNumber: *eventPayload.PullRequest.Number,
				Ref:      *eventPayload.PullRequest.Head.Ref,
				PrAuthor: *eventPayload.PullRequest.User.Login,
				PrLogger: prLogger,
				PrSHA:    *eventPayload.PullRequest.Head.SHA,
			}

			prMetadataRegex := regexp.MustCompile(`<!--\|.*\|(.*)\|-->`)
			serializedPrMetadata := prMetadataRegex.FindStringSubmatch(eventPayload.PullRequest.GetBody())
			if len(serializedPrMetadata) == 2 {
				if serializedPrMetadata[1] != "" {
					ghPrClientDetails.PrLogger.Info("Found PR metadata")
					err = ghPrClientDetails.PrMetadata.DeSerialize(serializedPrMetadata[1])
					if err != nil {
						ghPrClientDetails.PrLogger.Errorf("Fail to parser PR metadata %v", err)
					}
				}
			}

			githubapi.SetCommitStatus(ghPrClientDetails, "pending")
			var prHandleError error

			if *eventPayload.Action == "closed" && *eventPayload.PullRequest.Merged {
				err := handleMergedPrEvent(ghPrClientDetails, prApproverGithubClient)
				if err != nil {
					prHandleError = err
					ghPrClientDetails.PrLogger.Errorf("Handling of merged PR failed: err=%s\n", err)
				}
			} else if *eventPayload.Action == "opened" || *eventPayload.Action == "reopened" || *eventPayload.Action == "synchronize" {
				err = githubapi.MimizeStalePrComments(ghPrClientDetails, githubGraphQlClient)
				if err != nil {
					prHandleError = err
					log.Errorf("Failed to minimize stale comments: err=%s\n", err)
				}
				ghPrClientDetails.PrLogger.Infoln("Checking for Drift")
				err := promotion.DetectDrift(ghPrClientDetails)
				if err != nil {
					prHandleError = err
					ghPrClientDetails.PrLogger.Errorf("Drift detection failed: err=%s\n", err)
				}
			} else if *eventPayload.Action == "labeled" && githubapi.DoesPrHasLabel(*eventPayload, "show-plan") {
				ghPrClientDetails.PrLogger.Infoln("Found show-plan label, posting plan")
				defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
				config, _ := cfg.GetInRepoConfig(ghPrClientDetails, defaultBranch)
				promotions, _ := promotion.GeneratePromotionPlan(ghPrClientDetails, config, defaultBranch)
				commentPlanInPR(ghPrClientDetails, promotions)
			}

			if prHandleError == nil {
				githubapi.SetCommitStatus(ghPrClientDetails, "success")
			} else {
				githubapi.SetCommitStatus(ghPrClientDetails, "error")
			}

		case *github.IssueCommentEvent:
			log.Infof("Actionable event type %s\n", github.WebHookType(r))
			prLogger := log.WithFields(log.Fields{
				"repo":     *eventPayload.Repo.Owner.Login + "/" + *eventPayload.Repo.Name,
				"prNumber": *eventPayload.Issue.Number,
			})
			ghPrClientDetails := githubapi.GhPrClientDetails{
				Ctx:      ctx,
				Ghclient: mainGithubClient,
				Owner:    *eventPayload.Repo.Owner.Login,
				Repo:     *eventPayload.Repo.Name,
				PrNumber: *eventPayload.Issue.Number,
				PrAuthor: *eventPayload.Issue.User.Login,
				PrLogger: prLogger,
			}

			_ = handlecommentPrEvent(ghPrClientDetails, eventPayload)

		default:
			log.Infof("Non actionable event type %s\n", github.WebHookType(r))
			return
		}
	}
}

func handlecommentPrEvent(ghPrClientDetails githubapi.GhPrClientDetails, ce *github.IssueCommentEvent) error {
	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	config, err := cfg.GetInRepoConfig(ghPrClientDetails, defaultBranch)
	if err != nil {
		_ = ghPrClientDetails.CommentOnPr(fmt.Sprintf("Failed to get configuration\n```\n%s\n```\n", err))
		return err
	}
	// Comment events doesn't have Ref/SHA in payload, enriching the object:
	_, _ = ghPrClientDetails.GetRef()
	_, _ = ghPrClientDetails.GetSHA()
	for commentSubstring, commitStatusContext := range config.ToggleCommitStatus {
		if strings.Contains(*ce.Comment.Body, "/"+commentSubstring) {
			err := ghPrClientDetails.ToggleCommitStatus(commitStatusContext, *ce.Sender.Name)
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("Failed to toggle %s context,  err=%v", commitStatusContext, err)
				break
			} else {
				ghPrClientDetails.PrLogger.Infof("Toggled %s status", commitStatusContext)
			}
		}
	}
	return err
}

func handleMergedPrEvent(ghPrClientDetails githubapi.GhPrClientDetails, prApproverGithubClient *github.Client) error {
	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	config, err := cfg.GetInRepoConfig(ghPrClientDetails, defaultBranch)
	if err != nil {
		_ = ghPrClientDetails.CommentOnPr(fmt.Sprintf("Failed to get configuration\n```\n%s\n```\n", err))
		return err
	}

	// configBranch = default branch as the PR is closed at this and its branch deleted.
	// If we'l ever want to generate this plan on an unmerged PR the PR branch (ghPrClientDetails.Ref) should be used
	promotions, _ := promotion.GeneratePromotionPlan(ghPrClientDetails, config, defaultBranch)
	// log.Infof("%+v", promotions)
	if !config.DryRunMode {
		for _, promotion := range promotions {
			// TODO this whole part shouldn't be in main, but I need to refactor some circular dep's

			// because I use GitHub low level (tree) API the order of operation is somewhat different compared to regular git CLI flow:
			// I create the sync commit against HEAD, create a new branch based on that commit and finally open a PR based on that branch

			var treeEntries []*github.TreeEntry
			for trgt, src := range promotion.ComputedSyncPaths {
				err = githubapi.GenerateTreeEntiesForCommit(&treeEntries, ghPrClientDetails, src, trgt, defaultBranch)
				if err != nil {
					ghPrClientDetails.PrLogger.Errorf("Failed to generate treeEntries for %s > %s,  err=%v", src, trgt, err)
				} else {
					ghPrClientDetails.PrLogger.Debugf("Generated treeEntries for %s > %s", src, trgt)
				}
			}

			if len(treeEntries) < 1 {
				ghPrClientDetails.PrLogger.Infof("TreeEntries list is empty")
				continue
			}

			commit, err := githubapi.CreateSyncCommit(ghPrClientDetails, treeEntries, defaultBranch, promotion.Metadata.SourcePath)
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("Commit creation failed: err=%v", err)
				return err
			}
			newBranchRef, err := githubapi.CreateBranch(ghPrClientDetails, commit, promotion.Metadata.TargetPaths)
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("Branch creation failed: err=%v", err)
				return err
			}
			pull, err := githubapi.CreatePrObject(ghPrClientDetails, newBranchRef, promotion.Metadata.SourcePath, promotion.Metadata.TargetPaths, promotion.Metadata.ComponentNames, defaultBranch)
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("PR opening failed: err=%v", err)
				return err
			}
			if config.AutoApprovePromotionPrs {
				err := githubapi.ApprovePr(prApproverGithubClient, ghPrClientDetails, pull.Number)
				if err != nil {
					ghPrClientDetails.PrLogger.Errorf("PR auto approval failed: err=%v", err)
					return err
				}
			}
		}
	} else {
		commentPlanInPR(ghPrClientDetails, promotions)
	}
	return nil
}
