package githubapi

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/google/go-github/v48/github"
	log "github.com/sirupsen/logrus"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
)

type GhPrClientDetails struct {
	Ghclient *github.Client
	// This whole struct describe the metadata of the PR, so it makes sense to share the context with everything to generate HTTP calls related to that PR, right?
	Ctx           context.Context //nolint:containedctx
	DefaultBranch string
	Owner         string
	Repo          string
	PrAuthor      string
	PrNumber      int
	PrSHA         string
	Ref           string
	PrLogger      *log.Entry
	Labels        []*github.Label
}

func (p GhPrClientDetails) CommentOnPr(commentBody string) error {
	commentBody = "<!-- telefonistka_tag -->\n" + commentBody

	comment := &github.IssueComment{Body: &commentBody}
	_, resp, err := p.Ghclient.Issues.CreateComment(p.Ctx, p.Owner, p.Repo, p.PrNumber, comment)
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

	initialStatuses, resp, err := p.Ghclient.Repositories.ListStatuses(p.Ctx, p.Owner, p.Repo, p.Ref, listOpts)
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
				_, resp, err := p.Ghclient.Repositories.CreateStatus(p.Ctx, p.Owner, p.Repo, p.PrSHA, commitStatus)
				prom.InstrumentGhCall(resp)
				if err != nil {
					p.PrLogger.Errorf("Failed to create context %s, err=%s", context, err)
					r = err
				}
			} else {
				p.PrLogger.Infof("%s Toggled %s(%s) to failure", user, context, *commitStatus.State)
				*commitStatus.State = "failure"
				_, resp, err := p.Ghclient.Repositories.CreateStatus(p.Ctx, p.Owner, p.Repo, p.PrSHA, commitStatus)
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
	_, resp, err := ghPrClientDetails.Ghclient.Repositories.CreateStatus(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, ghPrClientDetails.PrSHA, commitStatus)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to set commit status: err=%s\n%v", err, resp)
	}
}

func (p *GhPrClientDetails) GetSHA() (string, error) {
	if p.PrSHA == "" {
		prObject, resp, err := p.Ghclient.PullRequests.Get(p.Ctx, p.Owner, p.Repo, p.PrNumber)
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
		prObject, resp, err := p.Ghclient.PullRequests.Get(p.Ctx, p.Owner, p.Repo, p.PrNumber)
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
		repo, resp, err := p.Ghclient.Repositories.Get(p.Ctx, p.Owner, p.Repo)
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
	_, directoryContent, resp, err := ghPrClientDetails.Ghclient.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *path, getContentOpts)
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

func GenerateTreeEntiesForCommit(treeEntries *[]*github.TreeEntry, ghPrClientDetails GhPrClientDetails, sourcePath string, targetPath string, defaultBranch string) error {
	repoContentGetOptions := github.RepositoryContentGetOptions{
		Ref: defaultBranch,
	}
	sourcePathSHA := ""

	// in GH API/go-github, to get directory SHA you need to scan the whole parent Dir 🤷
	_, directoryContent, resp, err := ghPrClientDetails.Ghclient.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, path.Dir(sourcePath), &repoContentGetOptions)
	prom.InstrumentGhCall(resp)
	if err != nil && resp.StatusCode != 404 {
		ghPrClientDetails.PrLogger.Errorf("Could not fetch source directory SHA err=%s\n%v\n", err, resp)
		return err
	} else if err == nil { // scaning the parent dir
		for _, dirElement := range directoryContent {
			if *dirElement.Path == sourcePath {
				sourcePathSHA = *dirElement.SHA
				break
			}
		}
	} // leaving out statusCode 404, this means the whole parent dir is missing, but the behavior is similar to the case we didn't find the dir

	if sourcePathSHA == "" {
		ghPrClientDetails.PrLogger.Infoln("Source directory wasn't found, assuming a deletion PR")
		err := generateDeletionTreeEntries(&ghPrClientDetails, &targetPath, &defaultBranch, treeEntries)
		if err != nil {
			ghPrClientDetails.PrLogger.Errorf("Failed to build deletion tree: err=%s\n", err)
			return err
		}
	} else {
		treeEntry := github.TreeEntry{
			Path: github.String(targetPath),
			Mode: github.String("040000"),
			Type: github.String("tree"),
			SHA:  github.String(sourcePathSHA),
		}
		*treeEntries = append(*treeEntries, &treeEntry)
	}

	return err
}

func CreateSyncCommit(ghPrClientDetails GhPrClientDetails, treeEntries []*github.TreeEntry, defaultBranch string, sourcePath string) (*github.Commit, error) {
	// To avoid cloning the repo locally, I'm using GitHub low level GIT Tree API to sync the source folder "over" the target folders
	// This works by getting the source dir git object SHA, and overwriting(Git.CreateTree) the target directory git object SHA with the source's SHA.

	ref, resp, err := ghPrClientDetails.Ghclient.Git.GetRef(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, "heads/"+defaultBranch)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to get main branch ref: err=%s\n", err)
		return nil, err
	}
	baseTreeSHA := ref.Object.SHA
	tree, resp, err := ghPrClientDetails.Ghclient.Git.CreateTree(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *baseTreeSHA, treeEntries)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to create Git Tree object: err=%s\n%+v", err, resp)
		return nil, err
	}
	parentCommit, resp, err := ghPrClientDetails.Ghclient.Git.GetCommit(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *baseTreeSHA)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to get parent commit: err=%s\n", err)
		return nil, err
	}

	newCommitConfig := &github.Commit{
		Message: github.String("Syncing from " + sourcePath),
		Parents: []*github.Commit{parentCommit},
		Tree:    tree,
		Author: &github.CommitAuthor{
			Name:  github.String("Telefonistka GitOps Bot"),
			Email: github.String("gitops-telefonistka@wayfair.com"), // TODO change these
		},
	}

	commit, resp, err := ghPrClientDetails.Ghclient.Git.CreateCommit(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, newCommitConfig)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to create Git commit: err=%s\n", err) // TODO comment this error to PR
		return nil, err
	}

	return commit, err
}

func CreateBranch(ghPrClientDetails GhPrClientDetails, commit *github.Commit, targetPaths []string) (string, error) {
	newBranchName := fmt.Sprintf("promotions/%v-%v-%v8", ghPrClientDetails.PrNumber, strings.Replace(ghPrClientDetails.Ref, "/", "-", -1), strings.Replace(strings.Join(targetPaths, "_"), "/", "-", -1)) // TODO max branch name length is 250 - make sure this fit
	newBranchRef := "refs/heads/" + newBranchName
	ghPrClientDetails.PrLogger.Infof("New branch name will be: %s", newBranchName)

	newRefGitObjct := &github.GitObject{
		SHA: commit.SHA,
	}

	newRefConfig := &github.Reference{
		Ref:    github.String(newBranchRef),
		Object: newRefGitObjct,
	}

	_, resp, err := ghPrClientDetails.Ghclient.Git.CreateRef(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, newRefConfig)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not create Git Ref: err=%s\n%v\n", err, resp)
		return "", err
	}
	return newBranchRef, err
}

func CreatePrObject(ghPrClientDetails GhPrClientDetails, newBranchRef string, targetPaths []string, componentNames []string, originalPrNumber int, defaultBranch string) (*github.PullRequest, error) {
	components := strings.Join(componentNames, ",")
	newPrTitle := fmt.Sprintf("🚀 Promotion: %s ➡️  %s", components, strings.Join(targetPaths, " "))
	var newPrBody string
	if len(targetPaths) == 1 {
		newPrBody = fmt.Sprintf("Promotion of PR #%v (`%s`) to: `%s`", originalPrNumber, components, targetPaths[0])
	} else {
		newPrBody = fmt.Sprintf("Promotion of PR #%v (`%s`) to:\n```\n%s\n```", originalPrNumber, components, strings.Join(targetPaths, "\n"))
	}

	newPrConfig := &github.NewPullRequest{
		Body:  github.String(newPrBody),
		Title: github.String(newPrTitle),
		Base:  github.String(defaultBranch),
		Head:  github.String(newBranchRef),
	}

	pull, resp, err := ghPrClientDetails.Ghclient.PullRequests.Create(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, newPrConfig)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not create GitHub PR: err=%s\n%v\n", err, resp)
		return nil, err
	} else {
		ghPrClientDetails.PrLogger.Infof("PR %d opened", *pull.Number)
	}

	prLables, resp, err := ghPrClientDetails.Ghclient.Issues.AddLabelsToIssue(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *pull.Number, []string{"promotion"})
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not label GitHub PR: err=%s\n%v\n", err, resp)
		return pull, err
	} else {
		ghPrClientDetails.PrLogger.Debugf("PR %v labeled\n%+v", pull.Number, prLables)
	}

	_, resp, err = ghPrClientDetails.Ghclient.Issues.AddAssignees(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *pull.Number, []string{ghPrClientDetails.PrAuthor})
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could set %s as assignee on PR,  err=%s", ghPrClientDetails.PrAuthor, err)
		return pull, err
	} else {
		ghPrClientDetails.PrLogger.Debugf(" %s was set as assignee on PR", ghPrClientDetails.PrAuthor)
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

func GetFileContent(ghPrClientDetails GhPrClientDetails, branch string, filePath string) (string, error) {
	rGetContentOps := github.RepositoryContentGetOptions{Ref: branch}
	fileContent, _, resp, err := ghPrClientDetails.Ghclient.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, filePath, &rGetContentOps)
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to get file:%s\n%v\n", err, resp)
		return "", err
	}
	fileContentString, err := fileContent.GetContent()
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Fail to serlize file:%s\n", err)
		return "", err
	}
	return fileContentString, nil
}
