package githubapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v62/github"
	lru "github.com/hashicorp/golang-lru/v2"
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

type GhClientPair struct {
	v3Client *github.Client
	v4Client *githubv4.Client
}

func getAppInstallationId(githubAppPrivateKeyPath string, githubAppId int64, githubRestAltURL string, ctx context.Context, owner string) (int64, error) {
	atr, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, githubAppId, githubAppPrivateKeyPath)
	if err != nil {
		panic(err)
	}
	tempClient := github.NewClient(
		&http.Client{
			Transport: atr,
			Timeout:   time.Second * 30,
		})

	if githubRestAltURL != "" {
		tempClient, err = tempClient.WithEnterpriseURLs(githubRestAltURL, githubRestAltURL)
		if err != nil {
			log.Fatalf("failed to create git client for app: %v\n", err)
		}
	}

	installations, _, err := tempClient.Apps.ListInstallations(ctx, &github.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list installations: %v\n", err)
	}

	var installID int64
	for _, i := range installations {
		if *i.Account.Login == owner {
			installID = i.GetID()
			log.Infof("Installation ID for GitHub Application # %v is: %v", githubAppId, installID)
			return installID, nil
		}
	}

	return 0, err
}

func createGithubAppRestClient(githubAppPrivateKeyPath string, githubAppId int64, githubAppInstallationId int64, githubRestAltURL string, ctx context.Context) *github.Client {
	itr, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, githubAppId, githubAppInstallationId, githubAppPrivateKeyPath)
	if err != nil {
		log.Fatal(err)
	}
	var client *github.Client

	if githubRestAltURL != "" {
		itr.BaseURL = githubRestAltURL
		client, _ = github.NewClient(&http.Client{Transport: itr}).WithEnterpriseURLs(githubRestAltURL, githubRestAltURL)
	} else {
		client = github.NewClient(&http.Client{Transport: itr})
	}
	return client
}

func createGithubRestClient(githubOauthToken string, githubRestAltURL string, ctx context.Context) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubOauthToken},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)
	if githubRestAltURL != "" {
		client, _ = client.WithEnterpriseURLs(githubRestAltURL, githubRestAltURL)
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

func createGhAppClientPair(ctx context.Context, githubAppId int64, owner string, ghAppPKeyPathEnvVarName string) GhClientPair {
	// var ghClientPair *GhClientPair
	var githubRestAltURL string
	var githubGraphqlAltURL string
	githubAppPrivateKeyPath := getCrucialEnv(ghAppPKeyPathEnvVarName)
	githubHost := getEnv("GITHUB_HOST", "")
	if githubHost != "" {
		githubRestAltURL = fmt.Sprintf("https://%s/api/v3", githubHost)
		githubGraphqlAltURL = fmt.Sprintf("https://%s/api/graphql", githubHost)
		log.Infof("Github REST API endpoint is configured to %s", githubRestAltURL)
		log.Infof("Github graphql API endpoint is configured to %s", githubGraphqlAltURL)
	} else {
		log.Debugf("Using public Github API endpoint")
	}

	githubAppInstallationId, err := getAppInstallationId(githubAppPrivateKeyPath, githubAppId, githubRestAltURL, ctx, owner)
	if err != nil {
		log.Errorf("Couldn't find installation for app ID %v and repo owner %s", githubAppId, owner)
	}

	// ghClientPair.v3Client := createGithubAppRestClient(githubAppPrivateKeyPath, githubAppId, githubAppInstallationId, githubRestAltURL, ctx)
	// ghClientPair.v4Client := createGithubAppGraphQlClient(githubAppPrivateKeyPath, githubAppId, githubAppInstallationId, githubGraphqlAltURL, githubRestAltURL, ctx)
	return GhClientPair{
		v3Client: createGithubAppRestClient(githubAppPrivateKeyPath, githubAppId, githubAppInstallationId, githubRestAltURL, ctx),
		v4Client: createGithubAppGraphQlClient(githubAppPrivateKeyPath, githubAppId, githubAppInstallationId, githubGraphqlAltURL, githubRestAltURL, ctx),
	}
}

func createGhTokenClientPair(ctx context.Context, ghOauthToken string) GhClientPair {
	// var ghClientPair *GhClientPair
	var githubRestAltURL string
	var githubGraphqlAltURL string
	githubHost := getEnv("GITHUB_HOST", "")
	if githubHost != "" {
		githubRestAltURL = fmt.Sprintf("https://%s/api/v3", githubHost)
		githubGraphqlAltURL = fmt.Sprintf("https://%s/api/graphql", githubHost)
		log.Infof("Github REST API endpoint is configured to %s", githubRestAltURL)
		log.Infof("Github graphql API endpoint is configured to %s", githubGraphqlAltURL)
	} else {
		log.Debugf("Using public Github API endpoint")
	}

	// ghClientPair.v3Client := createGithubRestClient(ghOauthToken, githubRestAltURL, ctx)
	// ghClientPair.v4Client := createGithubGraphQlClient(ghOauthToken, githubGraphqlAltURL)
	return GhClientPair{
		v3Client: createGithubRestClient(ghOauthToken, githubRestAltURL, ctx),
		v4Client: createGithubGraphQlClient(ghOauthToken, githubGraphqlAltURL),
	}
}

func (gcp *GhClientPair) GetAndCache(ghClientCache *lru.Cache[string, GhClientPair], ghAppIdEnvVarName string, ghAppPKeyPathEnvVarName string, ghOauthTokenEnvVarName string, repoOwner string, ctx context.Context) {
	githubAppId := getEnv(ghAppIdEnvVarName, "")
	var keyExist bool
	if githubAppId != "" {
		*gcp, keyExist = ghClientCache.Get(repoOwner)
		if keyExist {
			log.Debugf("Found cached client for %s", repoOwner)
		} else {
			log.Infof("Did not found cached client for %s, creating one with %s/%s env vars", repoOwner, ghAppIdEnvVarName, ghAppPKeyPathEnvVarName)
			githubAppIdint, err := strconv.ParseInt(githubAppId, 10, 64)
			if err != nil {
				log.Fatalf("GITHUB_APP_ID value could not converted to int64, %v", err)
			}
			*gcp = createGhAppClientPair(ctx, githubAppIdint, repoOwner, ghAppPKeyPathEnvVarName)
			ghClientCache.Add(repoOwner, *gcp)
		}
	} else {
		*gcp, keyExist = ghClientCache.Get("global")
		if keyExist {
			log.Debug("Found global cached client")
		} else {
			log.Infof("Did not found global cached client, creating one with %s env var", ghOauthTokenEnvVarName)
			ghOauthToken := getCrucialEnv(ghOauthTokenEnvVarName)

			*gcp = createGhTokenClientPair(ctx, ghOauthToken)
			ghClientCache.Add("global", *gcp)
		}
	}
}
