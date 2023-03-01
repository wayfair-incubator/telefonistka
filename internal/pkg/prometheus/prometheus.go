package prometheus

import (
	"strconv"
	"strings"

	"github.com/google/go-github/v48/github"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	webhookHitsVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:      "webhook_hits_total",
		Help:      "The total number of validated webhook hits",
		Namespace: "telefonistka",
		Subsystem: "webhook_server",
	}, []string{"parsing"})

	ghRateLimitCounter = promauto.NewGauge(prometheus.GaugeOpts{
		Name:      "github_rest_api_client_rate_limit",
		Help:      "The number of requests per hour the client is currently limited to",
		Namespace: "telefonistka",
		Subsystem: "github",
	})

	ghRateRemainingCounter = promauto.NewGauge(prometheus.GaugeOpts{
		Name:      "github_rest_api_client_rate_remaining",
		Help:      "The number of remaining requests the client can make this hour",
		Namespace: "telefonistka",
		Subsystem: "github",
	})

	GithubOpsCountVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name:      "github_operations_total",
		Help:      "The total number of Github API operations",
		Namespace: "telefonistka",
		Subsystem: "github",
	}, []string{"api_group", "api_path", "repo_slug", "status", "method"})
)

func InstrumentWebhookHit(parsing_status string) {
	webhookHitsVec.With(prometheus.Labels{"parsing": parsing_status}).Inc()
}

// [ repos AnOwner Arepo contents telefonistka.yaml]
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

	if len(relevantRequestPathSlice) < 4 {
		apiPath = ""
		if len(relevantRequestPathSlice) < 3 {
			repoSlug = ""
		} else {
			repoSlug = strings.Join(relevantRequestPathSlice[1:3], "/")
		}
	} else {
		apiPath = relevantRequestPathSlice[3]
		repoSlug = strings.Join(relevantRequestPathSlice[1:3], "/")
	}

	labels := prometheus.Labels{
		"api_group": relevantRequestPathSlice[0],
		"api_path":  apiPath,
		"repo_slug": repoSlug,
		"method":    resp.Request.Method,
		"status":    strconv.Itoa(resp.Response.StatusCode),
	}

	ghRateLimitCounter.Set(float64(resp.Rate.Limit))
	ghRateRemainingCounter.Set(float64(resp.Rate.Remaining))

	GithubOpsCountVec.With(labels).Inc()
	// resp.Request.

	return labels
}
