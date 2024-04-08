package configuration

import (
	"os"
	"testing"

	"github.com/go-test/deep"
)

func TestConfigurationParse(t *testing.T) {
	t.Parallel()

	configurationFileContent, _ := os.ReadFile("tests/testConfigurationParsing.yaml")

	config, err := ParseConfigFromYaml(string(configurationFileContent))
	if err != nil {
		t.Fatalf("config parsing failed: err=%s", err)
	}

	if config.PromotionPaths == nil {
		t.Fatalf("config is missing PromotionPaths, %v", config.PromotionPaths)
	}

	expectedConfig := &Config{
		PromotionPaths: []PromotionPath{
			{
				SourcePath: "workspace/",
				Conditions: Condition{
					PrHasLabels: []string{
						"some-label",
					},
					AutoMerge: true,
				},
				PromotionPrs: []PromotionPr{
					{
						TargetPaths: []string{
							"env/staging/us-east4/c1/",
						},
					},
					{
						TargetPaths: []string{
							"env/staging/europe-west4/c1/",
						},
					},
				},
			},
			{
				SourcePath: "env/staging/us-east4/c1/",
				Conditions: Condition{
					AutoMerge: false,
				},
				PromotionPrs: []PromotionPr{
					{
						TargetPaths: []string{
							"env/prod/us-central1/c2/",
						},
					},
				},
			},
			{
				SourcePath: "env/prod/us-central1/c2/",
				Conditions: Condition{
					AutoMerge: false,
				},
				PromotionPrs: []PromotionPr{
					{
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
