package telefonistka

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/alexliesenfeld/health"
	"github.com/google/go-github/v48/github"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/githubapi"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
)

var reverseCmd = &cobra.Command{
	Use:   "server",
	Short: "Runs the web server that listens to GitHub webhooks",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		serve()
	},
}

// This is still(https://github.com/spf13/cobra/issues/1862) the documented way to use cobra
func init() { //nolint:gochecknoinits
	rootCmd.AddCommand(reverseCmd)
}

func handleWebhook(mainGithubClient *github.Client, prApproverGithubClient *github.Client, githubGraphQlClient *githubv4.Client, ctx context.Context, githubWebhookSecret []byte) func(http.ResponseWriter, *http.Request) {
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

func serve() {
	switch getEnv("LOG_LEVEL", "info") {
	case "debug":
		log.SetLevel(log.DebugLevel)
		log.SetReportCaller(true)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	case "panic":
		log.SetLevel(log.PanicLevel)
	}

	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		// ForceColors: true,
		FullTimestamp: true,
	}) // TimestampFormat

	ctx := context.Background()

	var mainGithubClient *github.Client
	var githubGraphQlClient *githubv4.Client

	githubAppPrivateKeyPath := getEnv("GITHUB_APP_PRIVATE_KEY_PATH", "")
	githubHost := getEnv("GITHUB_HOST", "")
	var githubRestAltURL string
	var githubGraphqlAltURL string
	if githubHost != "" {
		githubRestAltURL = "https://" + githubHost + "/api/v3"
		githubGraphqlAltURL = "https://" + githubHost + "/api/graphql"
		log.Infof("Github REST API endpoint is configured to %s", githubRestAltURL)
		log.Infof("Github graphql API endpoint is configured to %s", githubGraphqlAltURL)
	} else {
		log.Infof("Using public Github API endpoint")
	}
	if githubAppPrivateKeyPath != "" {
		log.Infoln("Using GH app auth")

		githubAppId, err := strconv.ParseInt(getCrucialEnv("GITHUB_APP_ID"), 10, 64)
		if err != nil {
			log.Fatalf("GITHUB_APP_ID value could not converted to int64, %v", err)
		}

		mainGithubClient = githubapi.CreateGithubAppRestClient(githubAppPrivateKeyPath, githubAppId, githubRestAltURL, ctx)
		githubGraphQlClient = githubapi.CreateGithubAppGraphQlClient(githubAppPrivateKeyPath, githubAppId, githubGraphqlAltURL, githubRestAltURL, ctx)
	} else {
		mainGithubClient = githubapi.CreateGithubRestClient(getCrucialEnv("GITHUB_OAUTH_TOKEN"), githubRestAltURL, ctx)
		githubGraphQlClient = githubapi.CreateGithubGraphQlClient(getCrucialEnv("GITHUB_OAUTH_TOKEN"), githubGraphqlAltURL)
	}

	githubWebhookSecret := []byte(getCrucialEnv("GITHUB_WEBHOOK_SECRET"))
	prApproverGithubClient := githubapi.CreateGithubRestClient(getCrucialEnv("APPROVER_GITHUB_OAUTH_TOKEN"), githubRestAltURL, ctx)
	livenessChecker := health.NewChecker() // No checks for the moment, other then the http server availability
	readinessChecker := health.NewChecker(
		health.WithPeriodicCheck(30*time.Second, 0*time.Second, health.Check{
			// This is mainly meant to protect against a deployment with bad secret but could also allow monitoring on token expiry
			// A side benefit of this is that we can get an up-to-date  ratelimit usage metrics, at a relatively small waste of rate usage
			Name: "GitHub connectivity",
			Check: func(ctx context.Context) error {
				_, resp, err := mainGithubClient.APIMeta(ctx)
				prom.InstrumentGhCall(resp)
				if err != nil {
					log.Errorf("Liveness Check: Failed to access GH API:\nerr=%s\nresponse=%v", err, resp)
				} else {
					log.Debugln("Liveness Check: GH API check is OK")
				}
				return err
			},
			Timeout: 10 * time.Second,
		},
		),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", handleWebhook(mainGithubClient, prApproverGithubClient, githubGraphQlClient, ctx, githubWebhookSecret))
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/live", health.NewHandler(livenessChecker))
	mux.Handle("/ready", health.NewHandler(readinessChecker))

	srv := &http.Server{
		Handler:      mux,
		Addr:         ":8080",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Infoln("server started")
	log.Fatal(srv.ListenAndServe())
}
