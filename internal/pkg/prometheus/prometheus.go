package prometheus

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type PrCounters struct {
	OpenPrs           int
	OpenPromotionPrs  int
	PrWithStaleChecks int
}

var (
	webhookHitsVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:      "webhook_hits_total",
		Help:      "The total number of validated webhook hits",
		Namespace: "telefonistka",
		Subsystem: "webhook_server",
	}, []string{"parsing"})

	ghRateLimitCounter = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "github_rest_api_client_rate_limit",
		Help:      "The number of requests per hour the client is currently limited to",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"repo_owner"})

	ghRateRemainingCounter = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "github_rest_api_client_rate_remaining",
		Help:      "The number of remaining requests the client can make this hour",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"repo_owner"})

	githubOpsCountVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:      "github_operations_total",
		Help:      "The total number of Github API operations",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"api_group", "api_path", "repo_slug", "status", "method"})

	ghOpenPrsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "open_prs",
		Help:      "The total number of open PRs",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"repo_slug"})

	ghOpenPromotionPrsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "open_promotion_prs",
		Help:      "The total number of open PRs with promotion label",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"repo_slug"})

	ghOpenPrsWithPendingCheckGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name:      "open_prs_with_pending_telefonistka_checks",
		Help:      "The total number of open PRs with pending Telefonistka checks(excluding PRs with very recent commits)",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"repo_slug"})

	commitStatusUpdates = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:      "commit_status_updates_total",
		Help:      "The total number of commit status updates, and their status (success/pending/failure)",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"repo_slug", "status"})

	whUpstreamRequestsCountVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:      "upstream_requests_total",
		Help:      "The total number of requests forwarded upstream servers",
		Namespace: "telefonistka",
		Subsystem: "webhook_proxy",
	}, []string{"status", "method", "url"})
)

func IncCommitStatusUpdateCounter(repoSlug string, status string) {
	commitStatusUpdates.With(prometheus.Labels{
		"repo_slug": repoSlug,
		"status":    status,
	}).Inc()
}

func PublishPrMetrics(pc PrCounters, repoSlug string) {
	metricLables := prometheus.Labels{
		"repo_slug": repoSlug,
	}
	ghOpenPrsGauge.With(metricLables).Set(float64(pc.OpenPrs))
	ghOpenPromotionPrsGauge.With(metricLables).Set(float64(pc.OpenPromotionPrs))
	ghOpenPrsWithPendingCheckGauge.With(metricLables).Set(float64(pc.PrWithStaleChecks))
}

// This function instrument Webhook hits and parsing of their content
func InstrumentWebhookHit(parsing_status string) {
	webhookHitsVec.With(prometheus.Labels{"parsing": parsing_status}).Inc()
}

// This function instrument API calls to GitHub API
func InstrumentGhCall(resp *github.Response) prometheus.Labels {
	if resp == nil {
		return prometheus.Labels{}
	}
	requestPathSlice := strings.Split(resp.Request.URL.Path, "/")
	var relevantRequestPathSlice []string
	// GitHub enterprise API as an additional "api/v3" perfix
	if requestPathSlice[1] == "api" && requestPathSlice[2] == "v3" {
		relevantRequestPathSlice = requestPathSlice[3:]
	} else {
		relevantRequestPathSlice = requestPathSlice[1:]
	}
	var apiPath string
	var repoSlug string
	var repoOwner string

	if len(relevantRequestPathSlice) < 4 {
		apiPath = ""
		if len(relevantRequestPathSlice) < 3 {
			repoSlug = ""
			repoOwner = ""
		} else {
			repoSlug = strings.Join(relevantRequestPathSlice[1:3], "/")
			repoOwner = relevantRequestPathSlice[1]
		}
	} else {
		apiPath = relevantRequestPathSlice[3]
		repoSlug = strings.Join(relevantRequestPathSlice[1:3], "/")
		repoOwner = relevantRequestPathSlice[1]
	}

	labels := prometheus.Labels{
		"api_group": relevantRequestPathSlice[0],
		"api_path":  apiPath,
		"repo_slug": repoSlug,
		"method":    resp.Request.Method,
		"status":    strconv.Itoa(resp.Response.StatusCode),
	}

	rateLimitLables := prometheus.Labels{
		"repo_owner": repoOwner,
	}
	ghRateLimitCounter.With(rateLimitLables).Set(float64(resp.Rate.Limit))
	ghRateRemainingCounter.With(rateLimitLables).Set(float64(resp.Rate.Remaining))

	githubOpsCountVec.With(labels).Inc()
	// resp.Request.

	return labels
}

// This function instrument upstream webhooks for the WH forwarding/multiplexing feature
func InstrumentProxyUpstreamRequest(resp *http.Response) prometheus.Labels {
	if resp == nil {
		return prometheus.Labels{}
	}

	labels := prometheus.Labels{
		"method": resp.Request.Method,
		"status": strconv.Itoa(resp.StatusCode),
		"url":    resp.Request.URL.String(),
	}
	whUpstreamRequestsCountVec.With(labels).Inc()
	return labels
}
