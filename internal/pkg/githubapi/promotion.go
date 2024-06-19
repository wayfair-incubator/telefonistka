package githubapi

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/go-github/v62/github"
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
	TargetDescription              string
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


// This function generates a list of "components" that where changed in the PR and are relevant for promotion)
func generateListOfRelevantComponents(ghPrClientDetails GhPrClientDetails, config *cfg.Config) (relevantComponents map[relevantComponent]struct{}, err error) {
	relevantComponents = make(map[relevantComponent]struct{})

	// Get the list of files in the PR, with pagination
	opts := &github.ListOptions{}
	prFiles := []*github.CommitFile{}
	for {
		perPagePrFiles, resp, err := ghPrClientDetails.GhClientPair.v3Client.PullRequests.ListFiles(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, ghPrClientDetails.PrNumber, opts)
		prom.InstrumentGhCall(resp)
		if err != nil {
			ghPrClientDetails.PrLogger.Errorf("could not get file list from GH API: err=%s\nstatus code=%v", err, resp.Response.Status)

			return nil, err
		}
		prFiles = append(prFiles, perPagePrFiles...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	for _, changedFile := range prFiles {
		for _, promotionPathConfig := range config.PromotionPaths {
			if match, _ := regexp.MatchString("^"+promotionPathConfig.SourcePath+".*", *changedFile.Filename); match {
				// "components" here are the sub directories of the SourcePath
				// but with promotionPathConfig.ComponentPathExtraDepth we can grab multiple levels of subdirectories,
				// to support cases where components are nested deeper(e.g. [SourcePath]/owningTeam/namespace/component1)
				componentPathRegexSubSstrings := []string{}
				for i := 0; i <= promotionPathConfig.ComponentPathExtraDepth; i++ {
					componentPathRegexSubSstrings = append(componentPathRegexSubSstrings, "[^/]*")
				}
				componentPathRegexSubString := strings.Join(componentPathRegexSubSstrings, "/")
				getComponentRegexString := regexp.MustCompile("^" + promotionPathConfig.SourcePath + "(" + componentPathRegexSubString + ")/.*")
				componentName := getComponentRegexString.ReplaceAllString(*changedFile.Filename, "${1}")

				getSourcePathRegexString := regexp.MustCompile("^(" + promotionPathConfig.SourcePath + ")" + componentName + "/.*")
				compiledSourcePath := getSourcePathRegexString.ReplaceAllString(*changedFile.Filename, "${1}")
				relevantComponentsElement := relevantComponent{
					SourcePath:    compiledSourcePath,
					ComponentName: componentName,
					AutoMerge:     promotionPathConfig.Conditions.AutoMerge,
				}
				relevantComponents[relevantComponentsElement] = struct{}{}
				break // a file can only be a single "source dir"
			}
		}
	}
	return relevantComponents, nil
}

type relevantComponent struct {
	SourcePath    string
	ComponentName string
	AutoMerge     bool
}

// This function basically turns the map with struct keys into a list of strings
func generateListOfChangedComponentPaths(ghPrClientDetails GhPrClientDetails, config *cfg.Config) (changedComponentPaths []string, err error) {
	relevantComponents, err := generateListOfRelevantComponents(ghPrClientDetails, config)
	if err != nil {
		return nil, err
	}
	for component := range relevantComponents {
		changedComponentPaths = append(changedComponentPaths, component.SourcePath+component.ComponentName)
	}
	return changedComponentPaths, nil
}

// This function generates a promotion plan based on the list of relevant components that where "touched" and the in-repo telefonitka  configuration
func generatePlanBasedOnChangeddComponent(ghPrClientDetails GhPrClientDetails, config *cfg.Config, relevantComponents map[relevantComponent]struct{}, configBranch string) (promotions map[string]PromotionInstance, err error) {
	promotions = make(map[string]PromotionInstance)
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
						if ppr.TargetDescription == "" {
							ppr.TargetDescription = strings.Join(ppr.TargetPaths, " ")
						}
						promotions[mapKey] = PromotionInstance{
							Metadata: PromotionInstanceMetaData{
								TargetPaths:                    ppr.TargetPaths,
								TargetDescription:              ppr.TargetDescription,
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

func GeneratePromotionPlan(ghPrClientDetails GhPrClientDetails, config *cfg.Config, configBranch string) (map[string]PromotionInstance, error) {
	// TODO refactor tests to use the two functions below instead of this one
	relevantComponents, err := generateListOfRelevantComponents(ghPrClientDetails, config)
	if err != nil {
		return nil, err
	}
	promotions, err := generatePlanBasedOnChangeddComponent(ghPrClientDetails, config, relevantComponents, configBranch)
	return promotions, err
}
