package githubapi

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v52/github"
	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

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

func getAppInstallationId(githubAppPrivateKeyPath string, githubAppId int64, githubRestAltURL string, ctx context.Context) (int64, error) {
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
			log.Fatalf("failed to create git client for app: %v\n", err)
		}
	} else {
		tempClient = github.NewClient(
			&http.Client{
				Transport: atr,
				Timeout:   time.Second * 30,
			})
	}

	installations, _, err := tempClient.Apps.ListInstallations(ctx, &github.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list installations: %v\n", err)
	}

	installID := installations[0].GetID()
	log.Infof("Installation ID for GitHub Application # %v is: %v", githubAppId, installID)

	return installID, err
}

func createGithubAppRestClient(githubAppPrivateKeyPath string, githubAppId int64, githubAppInstallationId int64, githubRestAltURL string, ctx context.Context) *github.Client {
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, githubAppId, githubAppInstallationId, githubAppPrivateKeyPath)
	if err != nil {
		log.Fatal(err)
	}
	var client *github.Client

	if githubRestAltURL != "" {
		itr.BaseURL = githubRestAltURL
		client, _ = github.NewEnterpriseClient(githubRestAltURL, githubRestAltURL, &http.Client{Transport: itr})
	} else {
		client = github.NewClient(&http.Client{Transport: itr})
	}
	return client
}

func CreateGithubRestClient(githubOauthToken string, githubRestAltURL string, ctx context.Context) *github.Client {
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

func createGithubAppGraphQlClient(githubAppPrivateKeyPath string, githubAppId int64, githubAppInstallationId int64, githubGraphqlAltURL string, githubRestAltURL string, ctx context.Context) *githubv4.Client {
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, githubAppId, githubAppInstallationId, githubAppPrivateKeyPath)
	if err != nil {
		log.Fatal(err)
	}
	var client *githubv4.Client

	if githubGraphqlAltURL != "" {
		itr.BaseURL = githubRestAltURL
		client = githubv4.NewEnterpriseClient(githubGraphqlAltURL, &http.Client{Transport: itr})
	} else {
		client = githubv4.NewClient(&http.Client{Transport: itr})
	}
	return client
}

func createGithubGraphQlClient(githubOauthToken string, githubGraphqlAltURL string) *githubv4.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubOauthToken},
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

func CreateAllClients(ctx context.Context) (*github.Client, *githubv4.Client, *github.Client) {
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
		githubAppInstallationId, err := getAppInstallationId(githubAppPrivateKeyPath, githubAppId, githubRestAltURL, ctx)
		if err != nil {
			log.Fatalf("Could not get GitHub Application Installation ID: %v", err)
		}

		mainGithubClient = createGithubAppRestClient(githubAppPrivateKeyPath, githubAppId, githubAppInstallationId, githubRestAltURL, ctx)
		githubGraphQlClient = createGithubAppGraphQlClient(githubAppPrivateKeyPath, githubAppId, githubAppInstallationId, githubGraphqlAltURL, githubRestAltURL, ctx)
	} else {
		mainGithubClient = CreateGithubRestClient(getCrucialEnv("GITHUB_OAUTH_TOKEN"), githubRestAltURL, ctx)
		githubGraphQlClient = createGithubGraphQlClient(getCrucialEnv("GITHUB_OAUTH_TOKEN"), githubGraphqlAltURL)
	}

	prApproverGithubClient := CreateGithubRestClient(getCrucialEnv("APPROVER_GITHUB_OAUTH_TOKEN"), githubRestAltURL, ctx)

	return mainGithubClient, githubGraphQlClient, prApproverGithubClient
}
