package githubapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/go-github/v62/github"
	log "github.com/sirupsen/logrus"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/configuration"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
	"golang.org/x/exp/maps"
)

// @Title
// @Description
// @Author
// @Update

func generateListOfChangedFiles(eventPayload *github.PushEvent) []string {
	fileList := map[string]bool{} // using map for uniqueness

	for _, commit := range eventPayload.Commits {
		for _, file := range commit.Added {
			fileList[file] = true
		}
		for _, file := range commit.Modified {
			fileList[file] = true
		}
		for _, file := range commit.Removed {
			fileList[file] = true
		}
	}

	return maps.Keys(fileList)
}

func generateListOfEndpoints(listOfChangedFiles []string, config *configuration.Config) []string {
	endpoints := map[string]bool{} // using map for uniqueness
	for _, file := range listOfChangedFiles {
		for _, regex := range config.WebhookEndpointRegexs {
			m := regexp.MustCompile(regex.Expression)

			if m.MatchString(file) {
				for _, replacement := range regex.Replacements {
					endpoints[m.ReplaceAllString(file, replacement)] = true
				}
				break
			}
		}
	}

	return maps.Keys(endpoints)
}

func proxyRequest(ctx context.Context, originalHttpRequest *http.Request, body []byte, endpoint string, responses chan<- string) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, originalHttpRequest.Method, endpoint, bytes.NewBuffer(body))
	if err != nil {
		log.Errorf("Error creating request to %s: %v", endpoint, err)
		responses <- fmt.Sprintf("Failed to create request to %s", endpoint)
		return
	}
	req.Header = originalHttpRequest.Header.Clone()
	// Because payload and headers are passed as-is, I'm hoping webhook signature validation will "just work"

	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("Error proxying request to %s: %v", endpoint, err)
		responses <- fmt.Sprintf("Failed to proxy request to %s", endpoint)
		return
	} else {
		log.Debugf("Webhook successfully forwarded to %s", endpoint)
	}
	defer resp.Body.Close()

	_ = prom.InstrumentProxyUpstreamRequest(resp)

	respBody, err := io.ReadAll(resp.Body)

	if !strings.HasPrefix(resp.Status, "2") {
		log.Errorf("Got non 2XX HTTP status from  %s: status=%s body=%v", endpoint, resp.Status, body)
	}

	if err != nil {
		log.Errorf("Error reading response body from %s: %v", endpoint, err)
		responses <- fmt.Sprintf("Failed to read response from %s", endpoint)
		return
	}

	responses <- string(respBody)
}

func handlePushEvent(ctx context.Context, eventPayload *github.PushEvent, httpRequest *http.Request, payload []byte, ghPrClientDetails GhPrClientDetails) {
	listOfChangedFiles := generateListOfChangedFiles(eventPayload)
	log.Debugf("Changed files in push event: %v", listOfChangedFiles)

	defaultBranch := eventPayload.Repo.DefaultBranch

	if *eventPayload.Ref == "refs/heads/"+*defaultBranch {
		// TODO this need to be cached with TTL + invalidate if configfile in listOfChangedFiles?
		// This is possible because these webhooks are defined as "best effort" for the designed use case:
		// Speeding up ArgoCD reconcile loops
		config, _ := GetInRepoConfig(ghPrClientDetails, *defaultBranch)
		endpoints := generateListOfEndpoints(listOfChangedFiles, config)

		// Create a channel to receive responses from the goroutines
		responses := make(chan string)

		// Use a buffered channel with the same size as the number of endpoints
		// to prevent goroutines from blocking in case of slow endpoints
		results := make(chan string, len(endpoints))

		// Start a goroutine for each endpoint
		for _, endpoint := range endpoints {
			go proxyRequest(ctx, httpRequest, payload, endpoint, responses)
		}

		// Wait for all goroutines to finish and collect the responses
		for i := 0; i < len(endpoints); i++ {
			result := <-responses
			results <- result
		}

		close(responses)
		close(results)
	}
}
