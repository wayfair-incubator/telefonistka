package configuration

import (
	githubapi "github.com/wayfair-incubator/telefonistka/internal/pkg/githubapi"
	yaml "gopkg.in/yaml.v2"
)

type ComponentConfig struct {
	PromotionTargetAllowList []string `yaml:"promotionTargetAllowList"`
	PromotionTargetBlockList []string `yaml:"promotionTargetBlockList"`
}

type Condition struct {
	PrHasLabels []string `yaml:"prHasLabels"`
}

type PromotionPath struct {
	Conditions  Condition  `yaml:"conditions"`
	SourcePath  string     `yaml:"sourcePath"`
	TargetPaths [][]string `yaml:"targetPaths"`
}

type Config struct {
	// What paths trigger promotion to which paths
	PromotionPaths []PromotionPath `yaml:"promotionPaths"`

	// Generic configuration
	PromtionPrLables           []string          `yaml:"promtionPRlables"`
	PromotionBranchNameTemplte string            `yaml:"promotionBranchNameTemplte"`
	PromtionPrBodyTemplate     string            `yaml:"promtionPrBodyTemplate"`
	DryRunMode                 bool              `yaml:"dryRunMode"`
	AutoApprovePromotionPrs    bool              `yaml:"autoApprovePromotionPrs"`
	ToggleCommitStatus         map[string]string `yaml:"toggleCommitStatus"`
}

func GetInRepoConfig(ghPrClientDetails githubapi.GhPrClientDetails, defaultBranch string) (*Config, error) {
	// Create config structure
	config := &Config{}

	inRepoConfigFileContentString, err := githubapi.GetFileContent(ghPrClientDetails, defaultBranch, "telefonistka.yaml")
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Could not get in-repo configuration: err=%s\n", err)
		return nil, err
	}

	// Init new YAML decode
	err = yaml.Unmarshal([]byte(inRepoConfigFileContentString), config)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to parse configuration: err=%s\n", err) // TODO comment this error to PR
		return nil, err
	}

	return config, nil
}
