package githubapi

import (
	"context"
	"strings"

	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
)

// go-github is my peffered way to interact with GitHub because of the better developer expirience(pre made types, easy API mocking).
// But some functionality is not availalble in GH V3 rest API, like PR comment minimization, so here we are:
func GetBotGhIdentity(githubGraphQlClient *githubv4.Client, ctx context.Context) (string, error) {
	var getBotGhIdentityQuery struct {
		Viewer struct {
			Login githubv4.String
		}
	}

	err := githubGraphQlClient.Query(ctx, &getBotGhIdentityQuery, nil)
	botIdentity := getBotGhIdentityQuery.Viewer.Login
	if err != nil {
		log.Errorf("Failed to fetch token owner name: err=%s\n", err)
		return "", err
	}
	return string(botIdentity), nil
}

func MimizeStalePrComments(ghPrClientDetails GhPrClientDetails, githubGraphQlClient *githubv4.Client, botIdentity string) error {
	var getCommentNodeIdsQuery struct {
		Repository struct {
			PullRequest struct {
				Title    githubv4.String
				Comments struct {
					Edges []struct {
						Node struct {
							Id          githubv4.ID
							IsMinimized githubv4.Boolean
							Body        githubv4.String
							Author      struct {
								Login githubv4.String
							}
						}
					}
				} `graphql:"comments(last: 100)"`
			} `graphql:"pullRequest(number: $prNumber )"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	} // Mimizing stale comment is not crutial so only taking the last 100 comments, should cover most cases.
	// Would be nice if I could filter based on Author and isMinized here, in the query,  to get just the relevant ones,
	// but I don't think GH graphQL supports it, so for now I just filter in code, see conditioanl near the end of this function.

	getCommentNodeIdsParams := map[string]interface{}{
		"owner":    githubv4.String(ghPrClientDetails.Owner),
		"repo":     githubv4.String(ghPrClientDetails.Repo),
		"prNumber": githubv4.Int(ghPrClientDetails.PrNumber), //nolint:gosec // G115: type mismatch between shurcooL/githubv4 and google/go-github. Number taken from latter for use in query using former.
	}

	var minimizeCommentMutation struct {
		MinimizeComment struct {
			ClientMutationId githubv4.ID
			MinimizedComment struct {
				IsMinimized githubv4.Boolean
			}
		} `graphql:"minimizeComment(input: $input)"`
	}

	err := githubGraphQlClient.Query(ghPrClientDetails.Ctx, &getCommentNodeIdsQuery, getCommentNodeIdsParams)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to minimize stale comments: err=%s\n", err)
	}
	bi := githubv4.String(strings.TrimSuffix(botIdentity, "[bot]"))
	for _, prComment := range getCommentNodeIdsQuery.Repository.PullRequest.Comments.Edges {
		if !prComment.Node.IsMinimized && prComment.Node.Author.Login == bi {
			if strings.Contains(string(prComment.Node.Body), "<!-- telefonistka_tag -->") {
				ghPrClientDetails.PrLogger.Infof("Minimizing Comment %s", prComment.Node.Id)
				minimizeCommentInput := githubv4.MinimizeCommentInput{
					SubjectID:        prComment.Node.Id,
					Classifier:       githubv4.ReportedContentClassifiers("OUTDATED"),
					ClientMutationID: &bi,
				}
				err := githubGraphQlClient.Mutate(ghPrClientDetails.Ctx, &minimizeCommentMutation, minimizeCommentInput, nil)
				// As far as I can tell minimizeComment Github's grpahQL method doesn't accept list do doing one call per comment
				if err != nil {
					ghPrClientDetails.PrLogger.Errorf("Failed to minimize comment ID %s\n err=%s", prComment.Node.Id, err)
					// Handle error.
				}
			} else {
				ghPrClientDetails.PrLogger.Debugln("Ignoring comment without identification tag")
			}
		}
	}

	return err
}
