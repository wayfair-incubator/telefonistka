package telefonistka

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"

	lru "github.com/hashicorp/golang-lru/v2"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/githubapi"
)

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	var eventType string
	var eventFilePath string
	eventCmd := &cobra.Command{
		Use:   "event",
		Short: "Handles a GitHub event based on event JSON file",
		Long:  "Handles a GitHub event based on event JSON file.\nThis operation mode was was built with GitHub Actions in mind",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			event(eventType, eventFilePath)
		},
	}
	eventCmd.Flags().StringVarP(&eventType, "type", "t", getEnv("GITHUB_EVENT_NAME", ""), "Event type, defaults to GITHUB_EVENT_NAME env var")
	eventCmd.Flags().StringVarP(&eventFilePath, "file", "f", getEnv("GITHUB_EVENT_PATH", ""), "File path for event JSON, defaults to GITHUB_EVENT_PATH env var")
	rootCmd.AddCommand(eventCmd)
}

func event(eventType string, eventFilePath string) {
	ctx := context.Background()

	log.Infof("Event type: %s", eventType)
	log.Infof("Proccesing file: %s", eventFilePath)

	payload, err := os.ReadFile(eventFilePath)
	if err != nil {
		panic(err)
	}

	// To use the same code path as for Webhook I'm creating an http request with the payload from the file.
	// This might not be very smart.

	h, _ := http.NewRequest("POST", "", nil) //nolint:noctx
	h.Body = io.NopCloser(bytes.NewReader(payload))
	h.Header.Set("Content-Type", "application/json")
	h.Header.Set("X-GitHub-Event", eventType)

	mainGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	prApproverGhClientCache, _ := lru.New[string, githubapi.GhClientPair](128)
	githubapi.HandleEvent(h, ctx, mainGhClientCache, prApproverGhClientCache, nil)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
