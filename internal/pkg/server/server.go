package server

import (
	"context"
	"net/http"

	"github.com/google/go-github/v48/github"
	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
	githubapi "github.com/wayfair-incubator/telefonistka/internal/pkg/githubapi"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
)

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
		eventType := github.WebHookType(r)

		githubapi.HandleEvent(eventType, payload, mainGithubClient, prApproverGithubClient, githubGraphQlClient, ctx)
	}
}
