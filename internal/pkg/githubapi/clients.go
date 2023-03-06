package githubapi

import (
	"context"
	"net/http"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v48/github"
	"github.com/shurcooL/githubv4"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

func createGithubInstalltionHttpClient(githubAppPrivateKeyPath string, githubAppId int64, githubRestAltURL string, ctx context.Context) (*http.Client, error) {
	// GitHib app installation auth works as follows:
	// Use private key to generate JWT
	// Use JWT to in a temp new client to fetch "final" access token
	// Use new access token in a new token

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

func CreateGithubAppRestClient(githubAppPrivateKeyPath string, githubAppId int64, githubRestAltURL string, ctx context.Context) *github.Client {
	oauthClient, err := createGithubInstalltionHttpClient(githubAppPrivateKeyPath, githubAppId, githubRestAltURL, ctx)
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

func CreateGithubAppGraphQlClient(githubAppPrivateKeyPath string, githubAppId int64, githubGraphqlAltURL string, githubRestAltURL string, ctx context.Context) *githubv4.Client {
	httpClient, _ := createGithubInstalltionHttpClient(githubAppPrivateKeyPath, githubAppId, githubRestAltURL, ctx)
	var client *githubv4.Client
	if githubGraphqlAltURL != "" {
		client = githubv4.NewEnterpriseClient(githubGraphqlAltURL, httpClient)
	} else {
		client = githubv4.NewClient(httpClient)
	}
	return client
}

func CreateGithubGraphQlClient(githubOauthToken string, githubGraphqlAltURL string) *githubv4.Client {
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
