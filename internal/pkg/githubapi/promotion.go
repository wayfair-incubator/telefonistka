package githubapi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-github/v52/github"
	log "github.com/sirupsen/logrus"
	cfg "github.com/wayfair-incubator/telefonistka/internal/pkg/configuration"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
	yaml "gopkg.in/yaml.v2"
)

type PromotionInstance struct {
	Metadata          PromotionInstanceMetaData `deep:"-"` // Unit tests ignore Metadata currently
	ComputedSyncPaths map[string]string         // key is target, value is source
}

type PromotionInstanceMetaData struct {
	SourcePath                     string
	TargetPaths                    []string
	PerComponentSkippedTargetPaths map[string][]string // ComponentName is the key,
	ComponentNames                 []string
	AutoMerge                      bool
}

func containMatchingRegex(patterns []string, str string) bool {
	for _, pattern := range patterns {
		doesElementMatchPattern, err := regexp.MatchString(pattern, str)
		if err != nil {
			log.Errorf("failed to match regex %s vs %s\n%s", pattern, str, err)
			return false
		}
		if doesElementMatchPattern {
			return true
		}
	}
	return false
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func DetectDrift(ghPrClientDetails GhPrClientDetails) error {
	diffOutputMap := make(map[string]string)
	defaultBranch, _ := ghPrClientDetails.GetDefaultBranch()
	config, err := GetInRepoConfig(ghPrClientDetails, defaultBranch)
	if err != nil {
		_ = ghPrClientDetails.CommentOnPr(fmt.Sprintf("Failed to get configuration\n```\n%s\n```\n", err))
		return err
	}

	promotions, _ := GeneratePromotionPlan(ghPrClientDetails, config, ghPrClientDetails.Ref)

	for _, promotion := range promotions {
		ghPrClientDetails.PrLogger.Debugf("Checking drift for %s", promotion.Metadata.SourcePath)
		for trgt, src := range promotion.ComputedSyncPaths {
			hasDiff, diffOutput, _ := CompareRepoDirectories(ghPrClientDetails, src, trgt, defaultBranch)
			if hasDiff {
				mapKey := fmt.Sprintf("`%s` ↔️  `%s`", src, trgt)
				diffOutputMap[mapKey] = diffOutput
				ghPrClientDetails.PrLogger.Debugf("Found diff @ %s", mapKey)
			}
		}
	}
	if len(diffOutputMap) != 0 {
		err, templateOutput := executeTemplate(ghPrClientDetails.PrLogger, "driftMsg", "drift-pr-comment.gotmpl", diffOutputMap)
		if err != nil {
			return err
		}

		err = commentPR(ghPrClientDetails, templateOutput)
		if err != nil {
			return err
		}
	} else {
		ghPrClientDetails.PrLogger.Infof("No drift found")
	}

	return nil
}

func getComponentConfig(ghPrClientDetails GhPrClientDetails, componentPath string, branch string) (*cfg.ComponentConfig, error) {
	componentConfig := &cfg.ComponentConfig{}
	rGetContentOps := &github.RepositoryContentGetOptions{Ref: branch}
	componentConfigFileContent, _, resp, err := ghPrClientDetails.GhClientPair.v3Client.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, componentPath+"/telefonistka.yaml", rGetContentOps)
	prom.InstrumentGhCall(resp)
	if (err != nil) && (resp.StatusCode != 404) { // The file is optional
		ghPrClientDetails.PrLogger.Errorf("could not get file list from GH API: err=%s\nresponse=%v", err, resp)
		return nil, err
	} else if resp.StatusCode == 404 {
		ghPrClientDetails.PrLogger.Debugf("No in-component config in %s", componentPath)
		return nil, nil
	}
	componentConfigFileContentString, _ := componentConfigFileContent.GetContent()
	err = yaml.Unmarshal([]byte(componentConfigFileContentString), componentConfig)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Failed to parse configuration: err=%s\n", err) // TODO comment this error to PR
		return nil, err
	}
	return componentConfig, nil
}

func GeneratePromotionPlan(ghPrClientDetails GhPrClientDetails, config *cfg.Config, configBranch string) (map[string]PromotionInstance, error) {
	promotions := make(map[string]PromotionInstance)

	prFiles, resp, err := ghPrClientDetails.GhClientPair.v3Client.PullRequests.ListFiles(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, ghPrClientDetails.PrNumber, &github.ListOptions{})
	prom.InstrumentGhCall(resp)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("could not get file list from GH API: err=%s\nresponse=%v", err, resp)
		return promotions, err
	}

	//first we build a **unique** list of relevant directories
	//
	type relevantComponent struct {
		SourcePath    string
		ComponentName string
		AutoMerge     bool
	}
	relevantComponents := map[relevantComponent]bool{}

	for _, changedFile := range prFiles {
		for _, promotionPathConfig := range config.PromotionPaths {
			if match, _ := regexp.MatchString("^"+promotionPathConfig.SourcePath+".*", *changedFile.Filename); match {
				// "components" here are the sub directories of the SourcePath
				getComponentRegexString := regexp.MustCompile("^" + promotionPathConfig.SourcePath + "([^/]*)/.*")
				componentName := getComponentRegexString.ReplaceAllString(*changedFile.Filename, "${1}")

				getSourcePathRegexString := regexp.MustCompile("^(" + promotionPathConfig.SourcePath + ")" + componentName + "/.*")
				compiledSourcePath := getSourcePathRegexString.ReplaceAllString(*changedFile.Filename, "${1}")

				relevantComponentsElement := relevantComponent{
					SourcePath:    compiledSourcePath,
					ComponentName: componentName,
					AutoMerge:     promotionPathConfig.Conditions.AutoMerge,
				}
				relevantComponents[relevantComponentsElement] = true
				break // a file can only be a single "source dir"
			}
		}
	}

	// then we iterate over the list of relevant directories and generate a plan based on the configuration
	for componentToPromote := range relevantComponents {
		componentConfig, err := getComponentConfig(ghPrClientDetails, componentToPromote.SourcePath+componentToPromote.ComponentName, configBranch)
		if err != nil {
			ghPrClientDetails.PrLogger.Errorf("Failed to get in component configuration, err=%s\nskipping %s", err, componentToPromote.SourcePath+componentToPromote.ComponentName)
		}

		for _, configPromotionPath := range config.PromotionPaths {
			if match, _ := regexp.MatchString(configPromotionPath.SourcePath, componentToPromote.SourcePath); match {
				// This section checks if a PromotionPath has a condition and skips it if needed
				if configPromotionPath.Conditions.PrHasLabels != nil {
					thisPrHasTheRightLabel := false
					for _, l := range ghPrClientDetails.Labels {
						if contains(configPromotionPath.Conditions.PrHasLabels, *l.Name) {
							thisPrHasTheRightLabel = true
							break
						}
					}
					if !thisPrHasTheRightLabel {
						continue
					}
				}

				for _, ppr := range configPromotionPath.PromotionPrs {
					sort.Strings(ppr.TargetPaths)

					mapKey := configPromotionPath.SourcePath + ">" + strings.Join(ppr.TargetPaths, "|") // This key is used to aggregate the PR based on source and target combination
					if entry, ok := promotions[mapKey]; !ok {
						ghPrClientDetails.PrLogger.Debugf("Adding key %s", mapKey)
						promotions[mapKey] = PromotionInstance{
							Metadata: PromotionInstanceMetaData{
								TargetPaths:                    ppr.TargetPaths,
								SourcePath:                     componentToPromote.SourcePath,
								ComponentNames:                 []string{componentToPromote.ComponentName},
								PerComponentSkippedTargetPaths: map[string][]string{},
								AutoMerge:                      componentToPromote.AutoMerge,
							},
							ComputedSyncPaths: map[string]string{},
						}
					} else if !contains(entry.Metadata.ComponentNames, componentToPromote.ComponentName) {
						entry.Metadata.ComponentNames = append(entry.Metadata.ComponentNames, componentToPromote.ComponentName)
						promotions[mapKey] = entry
					}

					for _, indevidualPath := range ppr.TargetPaths {
						if componentConfig != nil {
							// BlockList supersedes Allowlist, if something matched there the entry is ignored regardless of allowlist
							if componentConfig.PromotionTargetBlockList != nil {
								if containMatchingRegex(componentConfig.PromotionTargetBlockList, indevidualPath) {
									promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName] = append(promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName], indevidualPath)
									continue
								}
							}
							if componentConfig.PromotionTargetAllowList != nil {
								if !containMatchingRegex(componentConfig.PromotionTargetAllowList, indevidualPath) {
									promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName] = append(promotions[mapKey].Metadata.PerComponentSkippedTargetPaths[componentToPromote.ComponentName], indevidualPath)
									continue
								}
							}
						}
						promotions[mapKey].ComputedSyncPaths[indevidualPath+componentToPromote.ComponentName] = componentToPromote.SourcePath + componentToPromote.ComponentName
					}
				}
				break
			}
		}
	}

	return promotions, nil
}
