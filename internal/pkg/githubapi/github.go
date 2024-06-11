package githubapi

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/google/go-github/v52/github"
	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/argocd"
	cfg "github.com/wayfair-incubator/telefonistka/internal/pkg/configuration"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
)

type promotionInstanceMetaData struct {
	SourcePath  string   `json:"sourcePath"`
	TargetPaths []string `json:"targetPaths"`
}

type GhPrClientDetails struct {
	GhClientPair *GhClientPair
	// This whole struct describe the metadata of the PR, so it makes sense to share the context with everything to generate HTTP calls related to that PR, right?
	Ctx           context.Context //nolint:containedctx
	DefaultBranch string
	Owner         string
	Repo          string
	PrAuthor      string
	PrNumber      int
	PrSHA         string
	Ref           string
	RepoURL       string
	PrLogger      *log.Entry
	Labels        []*github.Label
	PrMetadata    prMetadata
}

type prMetadata struct {
	OriginalPrAuthor          string                            `json:"originalPrAuthor"`
	OriginalPrNumber          int                               `json:"originalPrNumber"`
	PreviousPromotionMetadata map[int]promotionInstanceMetaData `json:"previousPromotionPaths"`
}

func (pm prMetadata) serialize() (string, error) {
	pmJson, err := json.Marshal(pm)
	if err != nil {
		return "", err
	}
	// var compressedPmJson []byte
	// _, err = lz4.CompressBlock(pmJson, compressedPmJson, nil)
	// if err != nil {
	// return "", err
	// }
	return base64.StdEncoding.EncodeToString(pmJson), nil
}

func HandlePREvent(eventPayload *github.PullRequestEvent, ghPrClientDetails GhPrClientDetails, mainGithubClientPair GhClientPair, approverGithubClientPair GhClientPair, ctx context.Context) {
	prMetadataRegex := regexp.MustCompile(`<!--\|.*\|(.*)\|-->`)
	serializedPrMetadata := prMetadataRegex.FindStringSubmatch(eventPayload.PullRequest.GetBody())
	if len(serializedPrMetadata) == 2 {
		if serializedPrMetadata[1] != "" {
			ghPrClientDetails.PrLogger.Info("Found PR metadata")
			err := ghPrClientDetails.PrMetadata.DeSerialize(serializedPrMetadata[1])
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("Fail to parser PR metadata %v", err)
			}
		}
	}

	// wasCommitStatusSet and the placement of SetCommitStatus in the flow is used to ensure an API call is only made where it needed
	wasCommitStatusSet := false

	var prHandleError error

	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	config, err := GetInRepoConfig(ghPrClientDetails, defaultBranch)
	if err != nil {
		ghPrClientDetails.PrLogger.Infof("Couldn't get Telefonistka in-repo configuration: %v", err)
	}

	if *eventPayload.Action == "closed" && *eventPayload.PullRequest.Merged {
		SetCommitStatus(ghPrClientDetails, "pending")
		wasCommitStatusSet = true
		err := handleMergedPrEvent(ghPrClientDetails, approverGithubClientPair.v3Client)
		if err != nil {
			prHandleError = err
			ghPrClientDetails.PrLogger.Errorf("Handling of merged PR failed: err=%s\n", err)
		}
	} else if *eventPayload.Action == "opened" || *eventPayload.Action == "reopened" || *eventPayload.Action == "synchronize" {
		SetCommitStatus(ghPrClientDetails, "pending")
		wasCommitStatusSet = true
		botIdentity, _ := GetBotGhIdentity(mainGithubClientPair.v4Client, ctx)
		err = MimizeStalePrComments(ghPrClientDetails, mainGithubClientPair.v4Client, botIdentity)
		if err != nil {
			prHandleError = err
			ghPrClientDetails.PrLogger.Errorf("Failed to minimize stale comments: err=%s\n", err)
		}
		if config.CommentArgocdDiffonPR {
			componentPathList, err := generateListOfChangedComponentPaths(ghPrClientDetails, config)
			if err != nil {
				prHandleError = err
				ghPrClientDetails.PrLogger.Errorf("Failed to get list of changed components: err=%s\n", err)
			}
			hasComponentDiff, hasComponentDiffErrors, diffOfChangedComponents, err := argocd.GenerateDiffOfChangedComponents(ctx, componentPathList, ghPrClientDetails.Ref, ghPrClientDetails.RepoURL)
			if err != nil {
				prHandleError = err
				ghPrClientDetails.PrLogger.Errorf("Failed to get ArgoCD diff information: err=%s\n", err)
			} else {
				ghPrClientDetails.PrLogger.Debugf("Successfully got ArgoCD diff\n")
				if !hasComponentDiffErrors && !hasComponentDiff {
					ghPrClientDetails.PrLogger.Debugf("ArgoCD diff is empty, this PR will not change cluster state\n")
					prLables, resp, err := ghPrClientDetails.GhClientPair.v3Client.Issues.AddLabelsToIssue(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *eventPayload.PullRequest.Number, []string{"noop"})
					prom.InstrumentGhCall(resp)
					if err != nil {
						ghPrClientDetails.PrLogger.Errorf("Could not label GitHub PR: err=%s\n%v\n", err, resp)
					} else {
						ghPrClientDetails.PrLogger.Debugf("PR %v labeled\n%+v", *eventPayload.PullRequest.Number, prLables)
					}
					if DoesPrHasLabel(*eventPayload, "promotion") && config.AutoMergeNoDiffPRs {
						ghPrClientDetails.PrLogger.Infof("Auto-merging (no diff) PR %d", *eventPayload.PullRequest.Number)
						err := MergePr(ghPrClientDetails, eventPayload.PullRequest.Number)
						if err != nil {
							prHandleError = err
							ghPrClientDetails.PrLogger.Errorf("PR auto merge failed: err=%v", err)
						}
					}
				}
			}

			err, templateOutput := executeTemplate(ghPrClientDetails.PrLogger, "argoCdDiff", "argoCD-diff-pr-comment.gotmpl", diffOfChangedComponents)
			if err != nil {
				prHandleError = err
				log.Errorf("Failed to generate ArgoCD diff comment template: err=%s\n", err)
			}
			err = commentPR(ghPrClientDetails, templateOutput)
			if err != nil {
				prHandleError = err
				log.Errorf("Failed to comment ArgoCD diff: err=%s\n", err)
			}
		}
		ghPrClientDetails.PrLogger.Infoln("Checking for Drift")
		err = DetectDrift(ghPrClientDetails)
		if err != nil {
			prHandleError = err
			ghPrClientDetails.PrLogger.Errorf("Drift detection failed: err=%s\n", err)
		}
	} else if *eventPayload.Action == "labeled" && DoesPrHasLabel(*eventPayload, "show-plan") {
		SetCommitStatus(ghPrClientDetails, "pending")
		wasCommitStatusSet = true
		ghPrClientDetails.PrLogger.Infoln("Found show-plan label, posting plan")
		promotions, _ := GeneratePromotionPlan(ghPrClientDetails, config, *eventPayload.PullRequest.Head.Ref)
		commentPlanInPR(ghPrClientDetails, promotions)
	}

	if wasCommitStatusSet {
		if prHandleError == nil {
			SetCommitStatus(ghPrClientDetails, "success")
		} else {
			SetCommitStatus(ghPrClientDetails, "error")
		}
	}
}

func HandleEvent(r *http.Request, ctx context.Context, mainGhClientCache *lru.Cache[string, GhClientPair], prApproverGhClientCache *lru.Cache[string, GhClientPair], githubWebhookSecret []byte) {
	payload, err := github.ValidatePayload(r, githubWebhookSecret)
	if err != nil {
		log.Errorf("error reading request body: err=%s\n", err)
		prom.InstrumentWebhookHit("validation_failed")
		return
	}
	eventType := github.WebHookType(r)

	eventPayloadInterface, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		log.Errorf("could not parse webhook: err=%s\n", err)
		prom.InstrumentWebhookHit("parsing_failed")
		return
	}
	prom.InstrumentWebhookHit("successful")
	var mainGithubClientPair GhClientPair
	var approverGithubClientPair GhClientPair

	switch eventPayload := eventPayloadInterface.(type) {
	case *github.PushEvent:
		// this is a commit push, do something with it?
		log.Infoln("is PushEvent")
		repoOwner := *eventPayload.Repo.Owner.Login
		mainGithubClientPair.GetAndCache(mainGhClientCache, "GITHUB_APP_ID", "GITHUB_APP_PRIVATE_KEY_PATH", "GITHUB_OAUTH_TOKEN", repoOwner, ctx)

		prLogger := log.WithFields(log.Fields{
			"event_type": "push",
		})

		ghPrClientDetails := GhPrClientDetails{
			Ctx:          ctx,
			GhClientPair: &mainGithubClientPair,
			Owner:        repoOwner,
			Repo:         *eventPayload.Repo.Name,
			RepoURL:      *eventPayload.Repo.HTMLURL,
			PrLogger:     prLogger,
		}

		handlePushEvent(ctx, eventPayload, r, payload, ghPrClientDetails)
	case *github.PullRequestEvent:
		log.Infof("is PullRequestEvent(%s)", *eventPayload.Action)

		prLogger := log.WithFields(log.Fields{
			"repo":     *eventPayload.Repo.Owner.Login + "/" + *eventPayload.Repo.Name,
			"prNumber": *eventPayload.PullRequest.Number,
		})

		repoOwner := *eventPayload.Repo.Owner.Login

		mainGithubClientPair.GetAndCache(mainGhClientCache, "GITHUB_APP_ID", "GITHUB_APP_PRIVATE_KEY_PATH", "GITHUB_OAUTH_TOKEN", repoOwner, ctx)
		approverGithubClientPair.GetAndCache(prApproverGhClientCache, "APPROVER_GITHUB_APP_ID", "APPROVER_GITHUB_APP_PRIVATE_KEY_PATH", "APPROVER_GITHUB_OAUTH_TOKEN", repoOwner, ctx)

		ghPrClientDetails := GhPrClientDetails{
			Ctx:          ctx,
			GhClientPair: &mainGithubClientPair,
			Labels:       eventPayload.PullRequest.Labels,
			Owner:        repoOwner,
			Repo:         *eventPayload.Repo.Name,
			RepoURL:      *eventPayload.Repo.HTMLURL,
			PrNumber:     *eventPayload.PullRequest.Number,
			Ref:          *eventPayload.PullRequest.Head.Ref,
			PrAuthor:     *eventPayload.PullRequest.User.Login,
			PrLogger:     prLogger,
			PrSHA:        *eventPayload.PullRequest.Head.SHA,
		}

		HandlePREvent(eventPayload, ghPrClientDetails, mainGithubClientPair, approverGithubClientPair, ctx)

	case *github.IssueCommentEvent:
		repoOwner := *eventPayload.Repo.Owner.Login
		mainGithubClientPair.GetAndCache(mainGhClientCache, "GITHUB_APP_ID", "GITHUB_APP_PRIVATE_KEY_PATH", "GITHUB_OAUTH_TOKEN", repoOwner, ctx)

		botIdentity, _ := GetBotGhIdentity(mainGithubClientPair.v4Client, ctx)
		log.Infof("Actionable event type %s\n", eventType)
		prLogger := log.WithFields(log.Fields{
			"repo":     *eventPayload.Repo.Owner.Login + "/" + *eventPayload.Repo.Name,
			"prNumber": *eventPayload.Issue.Number,
		})
		if *eventPayload.Comment.User.Login != botIdentity {
			ghPrClientDetails := GhPrClientDetails{
				Ctx:          ctx,
				GhClientPair: &mainGithubClientPair,
				Owner:        repoOwner,
				Repo:         *eventPayload.Repo.Name,
				RepoURL:      *eventPayload.Repo.HTMLURL,
				PrNumber:     *eventPayload.Issue.Number,
				PrAuthor:     *eventPayload.Issue.User.Login,
				PrLogger:     prLogger,
			}
			_ = handleCommentPrEvent(ghPrClientDetails, eventPayload)
		} else {
			log.Debug("Ignoring self comment")
		}

	default:
		log.Infof("Non-actionable event type %s", eventType)
		return
	}
}

func handleCommentPrEvent(ghPrClientDetails GhPrClientDetails, ce *github.IssueCommentEvent) error {
	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	config, err := GetInRepoConfig(ghPrClientDetails, defaultBranch)
	if err != nil {
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

func commentPlanInPR(ghPrClientDetails GhPrClientDetails, promotions map[string]PromotionInstance) {
	_, templateOutput := executeTemplate(ghPrClientDetails.PrLogger, "dryRunMsg", "dry-run-pr-comment.gotmpl", promotions)
	_ = commentPR(ghPrClientDetails, templateOutput)
}

func executeTemplate(logger *log.Entry, templateName string, templateFile string, data interface{}) (error, string) {
	var templateOutput bytes.Buffer
	messageTemplate, err := template.New(templateName).ParseFiles(getEnv("TEMPLATES_PATH", "templates/") + templateFile)
	if err != nil {
		logger.Errorf("Failed to parse template: err=%v", err)
		return err, ""
	}
	err = messageTemplate.ExecuteTemplate(&templateOutput, templateName, data)
	if err != nil {
		logger.Errorf("Failed to execute template: err=%v", err)
		return err, ""
	}
	return nil, templateOutput.String()
}

func commentPR(ghPrClientDetails GhPrClientDetails, commentBody string) error {
	err := ghPrClientDetails.CommentOnPr(commentBody)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to comment in PR: err=%v", err)
		return err
	}
	return nil
}

func BumpVersion(ghPrClientDetails GhPrClientDetails, defaultBranch string, filePath string, newFileContent string, triggeringRepo string, triggeringRepoSHA string, triggeringActor string, autoMerge bool) error {
	var treeEntries []*github.TreeEntry

	generateBumpTreeEntiesForCommit(&treeEntries, ghPrClientDetails, defaultBranch, filePath, newFileContent)

	commit, err := createCommit(ghPrClientDetails, treeEntries, defaultBranch, "Bumping version @ "+filePath)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Commit creation failed: err=%v", err)
		return err
	}
	newBranchRef, err := createBranch(ghPrClientDetails, commit, "artifact_version_bump/"+triggeringRepo+"/"+triggeringRepoSHA) // TODO figure out branch name!!!!
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Branch creation failed: err=%v", err)
		return err
	}

	newPrTitle := triggeringRepo + "🚠 Bumping version @ " + filePath
	newPrBody := fmt.Sprintf("Bumping version triggered by %s@%s", triggeringRepo, triggeringRepoSHA)
	pr, err := createPrObject(ghPrClientDetails, newBranchRef, newPrTitle, newPrBody, defaultBranch, triggeringActor)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("PR opening failed: err=%v", err)
		return err
	}

	ghPrClientDetails.PrLogger.Infof("New PR URL: %s", *pr.HTMLURL)

	if autoMerge {
		ghPrClientDetails.PrLogger.Infof("Auto-merging PR %d", *pr.Number)
		err := MergePr(ghPrClientDetails, pr.Number)
		if err != nil {
			ghPrClientDetails.PrLogger.Errorf("PR auto merge failed: err=%v", err)
			return err
		}
	}

	return nil
}

func handleMergedPrEvent(ghPrClientDetails GhPrClientDetails, prApproverGithubClient *github.Client) error {
	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	config, err := GetInRepoConfig(ghPrClientDetails, defaultBranch)
	if err != nil {
		_ = ghPrClientDetails.CommentOnPr(fmt.Sprintf("Failed to get configuration\n```\n%s\n```\n", err))
		return err
	}

	// configBranch = default branch as the PR is closed at this and its branch deleted.
	// If we'l ever want to generate this plan on an unmerged PR the PR branch (ghPrClientDetails.Ref) should be used
	promotions, _ := GeneratePromotionPlan(ghPrClientDetails, config, defaultBranch)
	// log.Infof("%+v", promotions)
	if !config.DryRunMode {
		for _, promotion := range promotions {
			// TODO this whole part shouldn't be in main, but I need to refactor some circular dep's

			// because I use GitHub low level (tree) API the order of operation is somewhat different compared to regular git CLI flow:
			// I create the sync commit against HEAD, create a new branch based on that commit and finally open a PR based on that branch

			var treeEntries []*github.TreeEntry
			for trgt, src := range promotion.ComputedSyncPaths {
				err = GenerateSyncTreeEntriesForCommit(&treeEntries, ghPrClientDetails, src, trgt, defaultBranch)
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

			commit, err := createCommit(ghPrClientDetails, treeEntries, defaultBranch, "Syncing from "+promotion.Metadata.SourcePath)
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("Commit creation failed: err=%v", err)
				return err
			}

			newBranchName := generateSafePromotionBranchName(ghPrClientDetails.PrNumber, ghPrClientDetails.Ref, promotion.Metadata.TargetPaths)
			newBranchRef, err := createBranch(ghPrClientDetails, commit, newBranchName)
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("Branch creation failed: err=%v", err)
				return err
			}

			components := strings.Join(promotion.Metadata.ComponentNames, ",")
			newPrTitle := fmt.Sprintf("🚀 Promotion: %s ➡️  %s", components, strings.Join(promotion.Metadata.TargetPaths, " "))

			var originalPrAuthor string
			// If the triggering PR was opened manually and it doesn't include in-body metadata, use the PR author
			// If the triggering PR as opened by Telefonistka and it has in-body metadata, fetch the original author from there
			if ghPrClientDetails.PrMetadata.OriginalPrAuthor != "" {
				originalPrAuthor = ghPrClientDetails.PrMetadata.OriginalPrAuthor
			} else {
				originalPrAuthor = ghPrClientDetails.PrAuthor
			}

			newPrBody := generatePromotionPrBody(ghPrClientDetails, components, promotion, originalPrAuthor)

			pull, err := createPrObject(ghPrClientDetails, newBranchRef, newPrTitle, newPrBody, defaultBranch, originalPrAuthor)
			if err != nil {
				ghPrClientDetails.PrLogger.Errorf("PR opening failed: err=%v", err)
				return err
			}
			if config.AutoApprovePromotionPrs {
				err := ApprovePr(prApproverGithubClient, ghPrClientDetails, pull.Number)
				if err != nil {
					ghPrClientDetails.PrLogger.Errorf("PR auto approval failed: err=%v", err)
					return err
				}
			}
			if promotion.Metadata.AutoMerge {
				ghPrClientDetails.PrLogger.Infof("Auto-merging PR %d", *pull.Number)
				templateData := map[string]interface{}{
					"prNumber": *pull.Number,
				}
				err, templateOutput := executeTemplate(ghPrClientDetails.PrLogger, "autoMerge", "auto-merge-comment.gotmpl", templateData)
				if err != nil {
					return err
				}
				err = commentPR(ghPrClientDetails, templateOutput)
				if err != nil {
					return err
				}

				err = MergePr(ghPrClientDetails, pull.Number)
				if err != nil {
					ghPrClientDetails.PrLogger.Errorf("PR auto merge failed: err=%v", err)
					return err
				}
			}
		}
	} else {
		commentPlanInPR(ghPrClientDetails, promotions)
	}
	return nil
}

// Creating a unique branch name based on the PR number, PR ref and the promotion target paths
// Max length of branch name is 250 characters
func generateSafePromotionBranchName(prNumber int, originalBranchName string, targetPaths []string) string {
	targetPathsBa := []byte(strings.Join(targetPaths, "_"))
	hasher := sha1.New()
	hasher.Write(targetPathsBa)
	uniqBranchNameSufix := firstN(hex.EncodeToString(hasher.Sum(nil)), 12)
	return fmt.Sprintf("promotions/%v-%v-%v", prNumber, firstN(strings.Replace(originalBranchName, "/", "-", -1), 200), uniqBranchNameSufix)
}

func firstN(str string, n int) string {
	v := []rune(str)
	if n >= len(v) {
		return str
	}
	return string(v[:n])
}

func MergePr(details GhPrClientDetails, number *int) error {
	_, resp, err := details.GhClientPair.v3Client.PullRequests.Merge(details.Ctx, details.Owner, details.Repo, *number, "Auto-merge", nil)
	prom.InstrumentGhCall(resp)
	if err != nil {
		details.PrLogger.Errorf("Failed to merge PR: err=%v", err)
	}
	return err
}

func (pm *prMetadata) DeSerialize(s string) error {
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	// _, err = lz4.UncompressBlock(decoded, unCompressedPmJson)
	// if err != nil {
	// return err
	// }
	err = json.Unmarshal(decoded, pm)
	return err
}

func (p GhPrClientDetails) CommentOnPr(commentBody string) error {
	commentBody = "<!-- telefonistka_tag -->\n" + commentBody

	comment := &github.IssueComment{Body: &commentBody}
	_, resp, err := p.GhClientPair.v3Client.Issues.CreateComment(p.Ctx, p.Owner, p.Repo, p.PrNumber, comment)
	prom.InstrumentGhCall(resp)
	if err != nil {
		p.PrLogger.Errorf("Could not comment in PR: err=%s\n%v\n", err, resp)
	}
	return err
}

func DoesPrHasLabel(eventPayload github.PullRequestEvent, name string) bool {
	result := false
	for _, prLabel := range eventPayload.PullRequest.Labels {
		if *prLabel.Name == name {
			result = true
			break
		}
	}
	return result
}

func (p *GhPrClientDetails) ToggleCommitStatus(context string, user string) error {
	var r error
	listOpts := &github.ListOptions{}

	initialStatuses, resp, err := p.GhClientPair.v3Client.Repositories.ListStatuses(p.Ctx, p.Owner, p.Repo, p.Ref, listOpts)
	prom.InstrumentGhCall(resp)
	if err != nil {
		p.PrLogger.Errorf("Failed to fetch  existing statuses for commit  %s, err=%s", p.Ref, err)
		r = err
	}

	for _, commitStatus := range initialStatuses {
		if *commitStatus.Context == context {
			if *commitStatus.State != "success" {
				p.PrLogger.Infof("%s Toggled  %s(%s) to success", user, context, *commitStatus.State)
				*commitStatus.State = "success"
				_, resp, err := p.GhClientPair.v3Client.Repositories.CreateStatus(p.Ctx, p.Owner, p.Repo, p.PrSHA, commitStatus)
				prom.InstrumentGhCall(resp)
				if err != nil {
					p.PrLogger.Errorf("Failed to create context %s, err=%s", context, err)
					r = err
				}
			} else {
				p.PrLogger.Infof("%s Toggled %s(%s) to failure", user, context, *commitStatus.State)
				*commitStatus.State = "failure"
				_, resp, err := p.GhClientPair.v3Client.Repositories.CreateStatus(p.Ctx, p.Owner, p.Repo, p.PrSHA, commitStatus)
				prom.InstrumentGhCall(resp)
				if err != nil {
					p.PrLogger.Errorf("Failed to create context %s, err=%s", context, err)
					r = err
				}
			}
			break
		}
	}

	return r
}

func SetCommitStatus(ghPrClientDetails GhPrClientDetails, state string) {
	// TODO change all these values
	context := "telefonistka"
	avatarURL := "https://avatars.githubusercontent.com/u/1616153?s=64"
	description := "Telefonistka GitOps Bot"
	targetURL := "https://github.com/wayfair-incubator/telefonistka"

	commitStatus := &github.RepoStatus{
		TargetURL:   &targetURL,
		Description: &description,
		State:       &state,
		Context:     &context,
		AvatarURL:   &avatarURL,
	}
	ghPrClientDetails.PrLogger.Debugf("Setting commit %s status to %s", ghPrClientDetails.PrSHA, state)
	_, resp, err := ghPrClientDetails.GhClientPair.v3Client.Repositories.CreateStatus(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, ghPrClientDetails.PrSHA, commitStatus)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to set commit status: err=%s\n%v", err, resp)
	}
}

func (p *GhPrClientDetails) GetSHA() (string, error) {
	if p.PrSHA == "" {
		prObject, resp, err := p.GhClientPair.v3Client.PullRequests.Get(p.Ctx, p.Owner, p.Repo, p.PrNumber)
		prom.InstrumentGhCall(resp)
		if err != nil {
			p.PrLogger.Errorf("Could not get pr data: err=%s\n%v\n", err, resp)
			return "", err
		}
		p.PrSHA = *prObject.Head.SHA
		return p.PrSHA, err
	} else {
		return p.PrSHA, nil
	}
}

func (p *GhPrClientDetails) GetRef() (string, error) {
	if p.Ref == "" {
		prObject, resp, err := p.GhClientPair.v3Client.PullRequests.Get(p.Ctx, p.Owner, p.Repo, p.PrNumber)
		prom.InstrumentGhCall(resp)
		if err != nil {
			p.PrLogger.Errorf("Could not get pr data: err=%s\n%v\n", err, resp)
			return "", err
		}
		p.Ref = *prObject.Head.Ref
		return p.Ref, err
	} else {
		return p.Ref, nil
	}
}

func (p *GhPrClientDetails) GetDefaultBranch() (string, error) {
	if p.DefaultBranch == "" {
		repo, resp, err := p.GhClientPair.v3Client.Repositories.Get(p.Ctx, p.Owner, p.Repo)
		if err != nil {
			p.PrLogger.Errorf("Could not get repo default branch: err=%s\n%v\n", err, resp)
			return "", err
		}
		prom.InstrumentGhCall(resp)
		p.DefaultBranch = *repo.DefaultBranch
		return *repo.DefaultBranch, err
	} else {
		return p.DefaultBranch, nil
	}
}

func generateDeletionTreeEntries(ghPrClientDetails *GhPrClientDetails, path *string, branch *string, treeEntries *[]*github.TreeEntry) error {
	// GH tree API doesn't allow deletion a whole dir, so this recursive function traverse the whole tree
	// and create a tree entry array that would delete all the files in that path
	getContentOpts := &github.RepositoryContentGetOptions{
		Ref: *branch,
	}
	_, directoryContent, resp, err := ghPrClientDetails.GhClientPair.v3Client.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *path, getContentOpts)
	prom.InstrumentGhCall(resp)
	if resp.StatusCode == 404 {
		ghPrClientDetails.PrLogger.Infof("Skipping deletion of non-existing  %s", *path)
		return nil
	} else if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not fetch %s content  err=%s\n%v\n", *path, err, resp)
		return err
	}
	for _, elementInDir := range directoryContent {
		if *elementInDir.Type == "file" {
			treeEntry := github.TreeEntry{ // https://docs.github.com/en/rest/git/trees?apiVersion=2022-11-28#create-a-tree
				Path:    github.String(*elementInDir.Path),
				Mode:    github.String("100644"),
				Type:    github.String("blob"),
				SHA:     nil,
				Content: nil,
			}
			*treeEntries = append(*treeEntries, &treeEntry)
		} else if *elementInDir.Type == "dir" {
			err := generateDeletionTreeEntries(ghPrClientDetails, elementInDir.Path, branch, treeEntries)
			if err != nil {
				return err
			}
		} else {
			ghPrClientDetails.PrLogger.Infof("Ignoring type %s for path %s", *elementInDir.Type, *elementInDir.Path)
		}
	}
	return nil
}

func generateBumpTreeEntiesForCommit(treeEntries *[]*github.TreeEntry, ghPrClientDetails GhPrClientDetails, defaultBranch string, filePath string, fileContent string) {
	treeEntry := github.TreeEntry{
		Path:    github.String(filePath),
		Mode:    github.String("100644"),
		Type:    github.String("blob"),
		Content: github.String(fileContent),
	}
	*treeEntries = append(*treeEntries, &treeEntry)
}

func getDirecotyGitObjectSha(ghPrClientDetails GhPrClientDetails, dirPath string, branch string) (string, error) {
	repoContentGetOptions := github.RepositoryContentGetOptions{
		Ref: branch,
	}

	direcotyGitObjectSha := ""
	// in GH API/go-github, to get directory SHA you need to scan the whole parent Dir 🤷
	_, directoryContent, resp, err := ghPrClientDetails.GhClientPair.v3Client.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, path.Dir(dirPath), &repoContentGetOptions)
	prom.InstrumentGhCall(resp)
	if err != nil && resp.StatusCode != 404 {
		ghPrClientDetails.PrLogger.Errorf("Could not fetch source directory SHA err=%s\n%v\n", err, resp)
		return "", err
	} else if err == nil { // scaning the parent dir
		for _, dirElement := range directoryContent {
			if *dirElement.Path == dirPath {
				direcotyGitObjectSha = *dirElement.SHA
				break
			}
		}
	} // leaving out statusCode 404, this means the whole parent dir is missing, but the behavior is similar to the case we didn't find the dir

	return direcotyGitObjectSha, nil
}

func GenerateSyncTreeEntriesForCommit(treeEntries *[]*github.TreeEntry, ghPrClientDetails GhPrClientDetails, sourcePath string, targetPath string, defaultBranch string) error {
	sourcePathSHA, err := getDirecotyGitObjectSha(ghPrClientDetails, sourcePath, defaultBranch)

	if sourcePathSHA == "" {
		ghPrClientDetails.PrLogger.Infoln("Source directory wasn't found, assuming a deletion PR")
		err := generateDeletionTreeEntries(&ghPrClientDetails, &targetPath, &defaultBranch, treeEntries)
		if err != nil {
			ghPrClientDetails.PrLogger.Errorf("Failed to build deletion tree: err=%s\n", err)
			return err
		}
	} else {
		syncTreeEntry := github.TreeEntry{
			Path: github.String(targetPath),
			Mode: github.String("040000"),
			Type: github.String("tree"),
			SHA:  github.String(sourcePathSHA),
		}
		*treeEntries = append(*treeEntries, &syncTreeEntry)

		// Aperntly... the way we sync directories(set the target dir git tree object SHA) doesn't delete files!!!! GH just "merges" the old and new tree objects.
		// So for now, I'll just go over all the files and add explicitly add  delete tree  entries  :(
		// TODO compare sourcePath targetPath Git object SHA to avoid costly tree compare where possible?
		sourceFilesSHAs := make(map[string]string)
		targetFilesSHAs := make(map[string]string)
		generateFlatMapfromFileTree(&ghPrClientDetails, &sourcePath, &sourcePath, &defaultBranch, sourceFilesSHAs)
		generateFlatMapfromFileTree(&ghPrClientDetails, &targetPath, &targetPath, &defaultBranch, targetFilesSHAs)

		for filename := range targetFilesSHAs {
			if _, found := sourceFilesSHAs[filename]; !found {
				ghPrClientDetails.PrLogger.Debugf("%s -- was NOT found on %s, marking as a deletion!", filename, sourcePath)
				fileDeleteTreeEntry := github.TreeEntry{
					Path:    github.String(targetPath + "/" + filename),
					Mode:    github.String("100644"),
					Type:    github.String("blob"),
					SHA:     nil, // this is how you delete a file https://docs.github.com/en/rest/git/trees?apiVersion=2022-11-28#create-a-tree
					Content: nil,
				}
				*treeEntries = append(*treeEntries, &fileDeleteTreeEntry)
			}
		}
	}

	return err
}

func createCommit(ghPrClientDetails GhPrClientDetails, treeEntries []*github.TreeEntry, defaultBranch string, commitMsg string) (*github.Commit, error) {
	// To avoid cloning the repo locally, I'm using GitHub low level GIT Tree API to sync the source folder "over" the target folders
	// This works by getting the source dir git object SHA, and overwriting(Git.CreateTree) the target directory git object SHA with the source's SHA.

	ref, resp, err := ghPrClientDetails.GhClientPair.v3Client.Git.GetRef(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, "heads/"+defaultBranch)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to get main branch ref: err=%s\n", err)
		return nil, err
	}
	baseTreeSHA := ref.Object.SHA
	tree, resp, err := ghPrClientDetails.GhClientPair.v3Client.Git.CreateTree(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *baseTreeSHA, treeEntries)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to create Git Tree object: err=%s\n%+v", err, resp)
		ghPrClientDetails.PrLogger.Errorf("These are the treeEntries: %+v", treeEntries)
		return nil, err
	}
	parentCommit, resp, err := ghPrClientDetails.GhClientPair.v3Client.Git.GetCommit(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *baseTreeSHA)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to get parent commit: err=%s\n", err)
		return nil, err
	}

	newCommitConfig := &github.Commit{
		Message: github.String(commitMsg),
		Parents: []*github.Commit{parentCommit},
		Tree:    tree,
	}

	commit, resp, err := ghPrClientDetails.GhClientPair.v3Client.Git.CreateCommit(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, newCommitConfig)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to create Git commit: err=%s\n", err) // TODO comment this error to PR
		return nil, err
	}

	return commit, err
}

func createBranch(ghPrClientDetails GhPrClientDetails, commit *github.Commit, newBranchName string) (string, error) {
	newBranchRef := "refs/heads/" + newBranchName
	ghPrClientDetails.PrLogger.Infof("New branch name will be: %s", newBranchName)

	newRefGitObjct := &github.GitObject{
		SHA: commit.SHA,
	}

	newRefConfig := &github.Reference{
		Ref:    github.String(newBranchRef),
		Object: newRefGitObjct,
	}

	_, resp, err := ghPrClientDetails.GhClientPair.v3Client.Git.CreateRef(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, newRefConfig)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not create Git Ref: err=%s\n%v\n", err, resp)
		return "", err
	}
	ghPrClientDetails.PrLogger.Infof("New branch ref: %s", newBranchRef)
	return newBranchRef, err
}

func generatePromotionPrBody(ghPrClientDetails GhPrClientDetails, components string, promotion PromotionInstance, originalPrAuthor string) string {
	// newPrMetadata will be serialized and persisted in the PR body for use when the PR is merged
	var newPrMetadata prMetadata
	var newPrBody string

	newPrMetadata.OriginalPrAuthor = originalPrAuthor

	if ghPrClientDetails.PrMetadata.PreviousPromotionMetadata != nil {
		newPrMetadata.PreviousPromotionMetadata = ghPrClientDetails.PrMetadata.PreviousPromotionMetadata
	} else {
		newPrMetadata.PreviousPromotionMetadata = make(map[int]promotionInstanceMetaData)
	}

	newPrMetadata.PreviousPromotionMetadata[ghPrClientDetails.PrNumber] = promotionInstanceMetaData{
		TargetPaths: promotion.Metadata.TargetPaths,
		SourcePath:  promotion.Metadata.SourcePath,
	}
	// newPrMetadata.PreviousPromotionMetadata[ghPrClientDetails.PrNumber].TargetPaths = targetPaths
	// newPrMetadata.PreviousPromotionMetadata[ghPrClientDetails.PrNumber].SourcePath = sourcePath

	newPrBody = fmt.Sprintf("Promotion path(%s):\n\n", components)

	keys := make([]int, 0)
	for k := range newPrMetadata.PreviousPromotionMetadata {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	sp := ""
	tp := ""
	for i, k := range keys {
		if len(newPrMetadata.PreviousPromotionMetadata[k].SourcePath) > 50 {
			sp = newPrMetadata.PreviousPromotionMetadata[k].SourcePath[:45] + "...✂️"
		} else {
			sp = newPrMetadata.PreviousPromotionMetadata[k].SourcePath
		}
		tp = "[" + strings.Join(newPrMetadata.PreviousPromotionMetadata[k].TargetPaths, ",") + "]"
		if len(tp) > 50 {
			tp = tp[:45] + "...✂️"
		}
		newPrBody = newPrBody + fmt.Sprintf("%s↘️  #%d  `%s` ➡️ `%s`\n", strings.Repeat("&nbsp;&nbsp;&nbsp;&nbsp;", i), k, sp, tp)
	}

	prMetadataString, _ := newPrMetadata.serialize()

	newPrBody = newPrBody + "\n<!--|Telefonistka data, do not delete|" + prMetadataString + "|-->"

	return newPrBody
}

func createPrObject(ghPrClientDetails GhPrClientDetails, newBranchRef string, newPrTitle string, newPrBody string, defaultBranch string, assignee string) (*github.PullRequest, error) {
	newPrConfig := &github.NewPullRequest{
		Body:  github.String(newPrBody),
		Title: github.String(newPrTitle),
		Base:  github.String(defaultBranch),
		Head:  github.String(newBranchRef),
	}

	pull, resp, err := ghPrClientDetails.GhClientPair.v3Client.PullRequests.Create(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, newPrConfig)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not create GitHub PR: err=%s\n%v\n", err, resp)
		return nil, err
	} else {
		ghPrClientDetails.PrLogger.Infof("PR %d opened", *pull.Number)
	}

	prLables, resp, err := ghPrClientDetails.GhClientPair.v3Client.Issues.AddLabelsToIssue(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *pull.Number, []string{"promotion"})
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not label GitHub PR: err=%s\n%v\n", err, resp)
		return pull, err
	} else {
		ghPrClientDetails.PrLogger.Debugf("PR %v labeled\n%+v", pull.Number, prLables)
	}

	_, resp, err = ghPrClientDetails.GhClientPair.v3Client.Issues.AddAssignees(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *pull.Number, []string{assignee})
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Warnf("Could not set %s as assignee on PR,  err=%s", assignee, err)
		// return pull, err
	} else {
		ghPrClientDetails.PrLogger.Debugf(" %s was set as assignee on PR", assignee)
	}

	// reviewers := github.ReviewersRequest{
	// Reviewers: []string{"SA-k8s-pr-approver-bot"}, // TODO remove hardcoding
	// }
	//
	// _, resp, err = ghPrClientDetails.Ghclient.PullRequests.RequestReviewers(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *pull.Number, reviewers)
	// prom.InstrumentGhCall(resp)
	// if err != nil {
	// ghPrClientDetails.PrLogger.Errorf("Could not set reviewer on pr: err=%s\n%v\n", err, resp)
	// return pull, err
	// } else {
	// ghPrClientDetails.PrLogger.Debugf("PR reviewer set.\n%+v", reviewers)
	// }

	return pull, nil // TODO
}

func ApprovePr(approverClient *github.Client, ghPrClientDetails GhPrClientDetails, prNumber *int) error {
	reviewRequest := &github.PullRequestReviewRequest{
		Event: github.String("APPROVE"),
	}

	_, resp, err := approverClient.PullRequests.CreateReview(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *prNumber, reviewRequest)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not create review: err=%s\n%v\n", err, resp)
		return err
	}

	return nil
}

func GetInRepoConfig(ghPrClientDetails GhPrClientDetails, defaultBranch string) (*cfg.Config, error) {
	inRepoConfigFileContentString, err, _ := GetFileContent(ghPrClientDetails, defaultBranch, "telefonistka.yaml")
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not get in-repo configuration: err=%s\n", err)
		return nil, err
	}
	c, err := cfg.ParseConfigFromYaml(inRepoConfigFileContentString)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to parse configuration: err=%s\n", err)
	}
	return c, err
}

func GetFileContent(ghPrClientDetails GhPrClientDetails, branch string, filePath string) (string, error, int) {
	rGetContentOps := github.RepositoryContentGetOptions{Ref: branch}
	fileContent, _, resp, err := ghPrClientDetails.GhClientPair.v3Client.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, filePath, &rGetContentOps)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to get file:%s\n%v\n", err, resp)
		return "", err, resp.StatusCode
	}
	fileContentString, err := fileContent.GetContent()
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to serlize file:%s\n", err)
		return "", err, resp.StatusCode
	}
	return fileContentString, nil, resp.StatusCode
}
