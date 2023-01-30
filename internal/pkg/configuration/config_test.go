package configuration

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-github/v48/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	log "github.com/sirupsen/logrus"
	githubapi "github.com/wayfair-incubator/telefonistka/internal/pkg/githubapi"
)

func TestConfigurationParse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	configurationFileContent, _ := os.ReadFile("tests/testConfigurationParsing.yaml")

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposContentsByOwnerByRepoByPath,
			github.RepositoryContent{
				Content: github.String(string(configurationFileContent)),
			},
		),
	)

	ghclient := github.NewClient(mockedHTTPClient)

	ghPrClientDetails := githubapi.GhPrClientDetails{
		Ctx:      ctx,
		Ghclient: ghclient,
		Owner:    "AnOwner",
		Repo:     "Arepo",
		PrNumber: 120,
		Ref:      "Abranch",
		PrLogger: log.WithFields(log.Fields{
			"repo":     "AnOwner/Arepo",
			"prNumber": 120,
		}),
	}
	config, err := GetInRepoConfig(ghPrClientDetails, "main")
	if err != nil {
		t.Fatalf("config parsing failed: err=%s", err)
	}

	if config.PromotionPaths == nil {
		t.Fatalf("config is missing PromotionPaths, %v", config.PromotionPaths)
	}
}
