package githubapi

import (
	"context"
	"time"

	"github.com/google/go-github/v62/github"
	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
)

const (
	timeToDefineStale = 20 * time.Minute
	metricRefreshTime = 60 * time.Second
)

func MainGhMetricsLoop(mainGhClientCache *lru.Cache[string, GhClientPair]) {
	for t := range time.Tick(metricRefreshTime) {
		log.Debugf("Updating pr metrics at %v", t)
		getPrMetrics(mainGhClientCache)
	}
}

func getRepoPrMetrics(ctx context.Context, ghClient GhClientPair, repo *github.Repository) (pc prom.PrCounters, err error) {
	log.Debugf("Checking repo %s", repo.GetName())
	ghOwner := repo.GetOwner().GetLogin()
	prListOpts := &github.PullRequestListOptions{
		State: "open",
	}
	prs := []*github.PullRequest{}

	// paginate through PRs, there might be lots of them.
	for {
		perPagePrs, resp, err := ghClient.v3Client.PullRequests.List(ctx, ghOwner, repo.GetName(), prListOpts)
		_ = prom.InstrumentGhCall(resp)
		if err != nil {
			log.Errorf("error getting PRs for %s/%s: %v", ghOwner, repo.GetName(), err)
		}
		prs = append(prs, perPagePrs...)
		if resp.NextPage == 0 {
			break
		}
		prListOpts.Page = resp.NextPage
	}

	for _, pr := range prs {
		if DoesPrHasLabel(pr.Labels, "promotion") {
			pc.OpenPromotionPrs++
		}

		log.Debugf("Checking PR %d", pr.GetNumber())
		commitStatuses, resp, err := ghClient.v3Client.Repositories.GetCombinedStatus(ctx, ghOwner, repo.GetName(), pr.GetHead().GetSHA(), nil)
		_ = prom.InstrumentGhCall(resp)
		if err != nil {
			log.Errorf("error getting statuses for %s/%s/%d: %v", ghOwner, repo.GetName(), pr.GetNumber(), err)
			continue
		}
		if isPrStalePending(commitStatuses, timeToDefineStale) {
			pc.PrWithStaleChecks++
		}
	}
	pc.OpenPrs = len(prs)

	return
}

// isPrStalePending checks if the a combinedStatus has a "telefonistka" context pending status that is older than timeToDefineStale and is in pending state
func isPrStalePending(commitStatuses *github.CombinedStatus, timeToDefineStale time.Duration) bool {
	for _, status := range commitStatuses.Statuses {
		if *status.Context == "telefonistka" &&
			*status.State == "pending" &&
			status.UpdatedAt.GetTime().Before(time.Now().Add(timeToDefineStale*-1)) {
			log.Debugf("Adding status %s-%v-%s !!!", *status.Context, status.UpdatedAt.GetTime(), *status.State)
			return true
		} else {
			log.Debugf("Ignoring status %s-%v-%s", *status.Context, status.UpdatedAt.GetTime(), *status.State)
		}
	}

	return false
}

// getPrMetrics iterates through all clients , gets all repos and then all PRs and calculates metrics
// getPrMetrics assumes Telefonsitka uses a GitHub App style of authentication as it uses the Apps.ListRepos call
// When using  personal access token authentication, Telefonistka is unaware of the "relevant" repos (at least it get a webhook from them).
func getPrMetrics(mainGhClientCache *lru.Cache[string, GhClientPair]) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	for _, ghOwner := range mainGhClientCache.Keys() {
		log.Debugf("Checking gh Owner %s", ghOwner)
		ghClient, _ := mainGhClientCache.Get(ghOwner)
		repos, resp, err := ghClient.v3Client.Apps.ListRepos(ctx, nil)
		_ = prom.InstrumentGhCall(resp)
		if err != nil {
			log.Errorf("error getting repos for %s: %v", ghOwner, err)
			continue
		}
		for _, repo := range repos.Repositories {
			pc, err := getRepoPrMetrics(ctx, ghClient, repo)
			if err != nil {
				log.Errorf("error getting repos for %s: %v", ghOwner, err)
				continue
			}
			prom.PublishPrMetrics(pc, repo.GetFullName())
		}
	}
}
