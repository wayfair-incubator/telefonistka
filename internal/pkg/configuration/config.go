package configuration

import (
	yaml "gopkg.in/yaml.v2"
)

type WebhookEndpointRegex struct {
	Expression   string   `yaml:"expression"`
	Replacements []string `yaml:"replacements"`
}

type ComponentConfig struct {
	PromotionTargetAllowList []string `yaml:"promotionTargetAllowList"`
	PromotionTargetBlockList []string `yaml:"promotionTargetBlockList"`
}

type Condition struct {
	PrHasLabels []string `yaml:"prHasLabels"`
	AutoMerge   bool     `yaml:"autoMerge"`
}

type PromotionPr struct {
	TargetPaths []string `yaml:"targetPaths"`
}

type PromotionPath struct {
	Conditions   Condition     `yaml:"conditions"`
	SourcePath   string        `yaml:"sourcePath"`
	PromotionPrs []PromotionPr `yaml:"promotionPrs"`
}

type Config struct {
	// What paths trigger promotion to which paths
	PromotionPaths []PromotionPath `yaml:"promotionPaths"`

	// Generic configuration
	PromtionPrLables        []string               `yaml:"promtionPRlables"`
	DryRunMode              bool                   `yaml:"dryRunMode"`
	AutoApprovePromotionPrs bool                   `yaml:"autoApprovePromotionPrs"`
	ToggleCommitStatus      map[string]string      `yaml:"toggleCommitStatus"`
	WebhookEndpointRegexs   []WebhookEndpointRegex `yaml:"webhookEndpointRegexs"`
}

func ParseConfigFromYaml(y string) (*Config, error) {
	config := &Config{}

	err := yaml.Unmarshal([]byte(y), config)

	return config, err
}
