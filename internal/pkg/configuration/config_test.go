package configuration

import (
	"context"
	"os"
	"testing"

	"github.com/go-test/deep"
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

	expectedConfig := &Config{
		PromotionPaths: []PromotionPath{
			PromotionPath{
				SourcePath: "workspace/",
				Conditions: Condition{
					PrHasLabels: []string{
						"some-label",
					},
				},
				PromotionPrs: []PromotionPr{
					PromotionPr{
						TargetPaths: []string{
							"env/staging/us-east4/c1/",
						},
					},
					PromotionPr{

						TargetPaths: []string{
							"env/staging/europe-west4/c1/",
						},
					},
				},
			},
			PromotionPath{
				SourcePath: "env/staging/us-east4/c1/",
				PromotionPrs: []PromotionPr{
					PromotionPr{
						TargetPaths: []string{
							"env/prod/us-central1/c2/",
						},
					},
				},
			},
			PromotionPath{
				SourcePath: "env/prod/us-central1/c2/",
				PromotionPrs: []PromotionPr{
					PromotionPr{
						TargetPaths: []string{
							"env/prod/us-west1/c2/",
							"env/prod/us-central1/c3/",
						},
					},
				},
			},
		},
	}

	if diff := deep.Equal(expectedConfig, config); diff != nil {
		t.Error(diff)
	}
}
