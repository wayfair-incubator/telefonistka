package main

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/alexliesenfeld/health"
	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v48/github"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
	server "github.com/wayfair-incubator/telefonistka/internal/pkg/server"
	"golang.org/x/oauth2"
)

func createGithubInstalltionHttpClient(githubAppPrivateKeyPath string, githubRestAltURL string, ctx context.Context) (*http.Client, error) {
	// GitHib app installation auth works as follows:
	// Use private key to generate JWT
	// Use JWT to in a temp new client to fetch "final" access token
	// Use new access token in a new token

	githubAppId, err := strconv.ParseInt(getCrucialEnv("GITHUB_APP_ID"), 10, 64)
	if err != nil {
		log.Fatalf("GITHUB_APP_ID value could not converted to int64, %v", err)
	}

	atr, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, githubAppId, githubAppPrivateKeyPath)
	if err != nil {
		panic(err)
	}
	var tempClient *github.Client

	if githubRestAltURL != "" {
		tempClient, err = github.NewEnterpriseClient(
			githubRestAltURL,
			githubRestAltURL,
			&http.Client{
				Transport: atr,
				Timeout:   time.Second * 30,
			})
		if err != nil {
			log.Fatalf("faild to create git client for app: %v\n", err)
		}
	} else {
		tempClient = github.NewClient(
			&http.Client{
				Transport: atr,
				Timeout:   time.Second * 30,
			})
	}

	installations, _, err := tempClient.Apps.ListInstallations(context.Background(), &github.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list installations: %v\n", err)
	}

	var installID int64
	for _, val := range installations {
		installID = val.GetID() // TODO how would this work on multiple installs????!?
	}

	log.Infoln(installID)

	token, _, err := tempClient.Apps.CreateInstallationToken(
		ctx,
		installID,
		&github.InstallationTokenOptions{})
	if err != nil {
		log.Fatalf("failed to create installation token: %v\n", err)
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token.GetToken()},
	)

	return oauth2.NewClient(context.Background(), ts), nil
}

func createGithubAppRestClient(githubAppPrivateKeyPath string, githubRestAltURL string, ctx context.Context) *github.Client {
	oauthClient, err := createGithubInstalltionHttpClient(githubAppPrivateKeyPath, githubRestAltURL, ctx)
	var finalClient *github.Client
	if githubRestAltURL != "" {
		finalClient, err = github.NewEnterpriseClient(githubRestAltURL, githubRestAltURL, oauthClient)
		if err != nil {
			log.Fatalf("faild to create git client for app: %v\n", err)
		}
	} else {
		finalClient = github.NewClient(oauthClient)
	}

	if err != nil {
		log.Fatalf("failed to create new git client with token: %v\n", err)
	}
	return finalClient
}

func createGithubRestClient(tokenEnvVarName string, githubRestAltURL string, ctx context.Context) *github.Client {
	githubOauthToken := getCrucialEnv(tokenEnvVarName)

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubOauthToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	var client *github.Client
	if githubRestAltURL != "" {
		client, _ = github.NewEnterpriseClient(githubRestAltURL, githubRestAltURL, tc)
	} else {
		client = github.NewClient(tc)
	}

	return client
}

func createGithubAppGraphQlClient(githubAppPrivateKeyPath string, githubGraphqlAltURL string, ctx context.Context) *githubv4.Client {
	httpClient, _ := createGithubInstalltionHttpClient(githubAppPrivateKeyPath, githubGraphqlAltURL, ctx)
	var client *githubv4.Client
	if githubGraphqlAltURL != "" {
		client = githubv4.NewEnterpriseClient(githubGraphqlAltURL, httpClient)
	} else {
		client = githubv4.NewClient(httpClient)
	}
	return client
}

func createGithubGraphQlClient(tokenEnvVarName string, githubGraphqlAltURL string) *githubv4.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: getCrucialEnv(tokenEnvVarName)},
	)
	httpClient := oauth2.NewClient(context.Background(), ts)
	var client *githubv4.Client
	if githubGraphqlAltURL != "" {
		client = githubv4.NewEnterpriseClient(githubGraphqlAltURL, httpClient)
	} else {
		client = githubv4.NewClient(httpClient)
	}
	return client
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getCrucialEnv(key string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	log.Fatalf("%s environment variable is required", key)
	os.Exit(3)
	return ""
}

func main() {
	switch getEnv("LOG_LEVEL", "info") {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
		log.SetReportCaller(true)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logrus.SetLevel(logrus.FatalLevel)
	case "panic":
		logrus.SetLevel(logrus.PanicLevel)
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
		githubRestAltURL = githubHost + "/api/v3"
		githubGraphqlAltURL = githubHost + "api/graphql"
		log.Infof("Github REST API endpoint is configured to %s", githubRestAltURL)
	} else {
		log.Infof("Using public Github API endpoint")
	}
	if githubAppPrivateKeyPath != "" {
		log.Infoln("Using GH app auth")
		mainGithubClient = createGithubAppRestClient(githubAppPrivateKeyPath, githubRestAltURL, ctx)
		githubGraphQlClient = createGithubAppGraphQlClient(githubAppPrivateKeyPath, githubGraphqlAltURL, ctx)
	} else {
		mainGithubClient = createGithubRestClient("GITHUB_OAUTH_TOKEN", githubRestAltURL, ctx)
		githubGraphQlClient = createGithubGraphQlClient("GITHUB_OAUTH_TOKEN", githubGraphqlAltURL)
	}

	githubWebhookSecret := []byte(getCrucialEnv("GITHUB_WEBHOOK_SECRET"))
	prApproverGithubClient := createGithubRestClient("APPROVER_GITHUB_OAUTH_TOKEN", githubRestAltURL, ctx)
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
	mux.HandleFunc("/webhook", server.HandleWebhook(mainGithubClient, prApproverGithubClient, githubGraphQlClient, ctx, githubWebhookSecret))
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
