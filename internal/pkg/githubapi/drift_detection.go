package githubapi

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	prom "github.com/wayfair-incubator/telefonistka/internal/pkg/prometheus"
)

func generateDiffOutput(ghPrClientDetails GhPrClientDetails, defaultBranch string, sourceFilesSHAs map[string]string, targetFilesSHAs map[string]string, sourcePath string, targetPath string) (bool, string, error) {
	var hasDiff bool
	var diffOutput bytes.Buffer
	var filesWithDiff []string
	diffOutput.WriteString("\n```diff\n")

	// staring with collecting files with different content and file only present in the source dir
	for filename, sha := range sourceFilesSHAs {
		ghPrClientDetails.PrLogger.Debugf("Looking at file %s", filename)
		if targetPathfileSha, found := targetFilesSHAs[filename]; found {
			if sha != targetPathfileSha {
				ghPrClientDetails.PrLogger.Debugf("%s is different from %s", sourcePath+"/"+filename, targetPath+"/"+filename)
				hasDiff = true
				sourceFileContent, _, _ := GetFileContent(ghPrClientDetails, defaultBranch, sourcePath+"/"+filename)
				targetFileContent, _, _ := GetFileContent(ghPrClientDetails, defaultBranch, targetPath+"/"+filename)

				edits := myers.ComputeEdits(span.URIFromPath(filename), sourceFileContent, targetFileContent)
				diffOutput.WriteString(fmt.Sprint(gotextdiff.ToUnified(sourcePath+"/"+filename, targetPath+"/"+filename, sourceFileContent, edits)))
				filesWithDiff = append(filesWithDiff, sourcePath+"/"+filename)
			} else {
				ghPrClientDetails.PrLogger.Debugf("%s is identical to  %s", sourcePath+"/"+filename, targetPath+"/"+filename)
			}
		} else {
			hasDiff = true
			diffOutput.WriteString(fmt.Sprintf("--- %s/%s (missing from target dir %s)\n", sourcePath, filename, targetPath))
		}
	}

	// then going over the target to check files that only exists there
	for filename := range targetFilesSHAs {
		if _, found := sourceFilesSHAs[filename]; !found {
			diffOutput.WriteString(fmt.Sprintf("+++ %s/%s (missing from source dir %s)\n", targetPath, filename, sourcePath))
			hasDiff = true
		}
	}

	diffOutput.WriteString("\n```\n")

	if len(filesWithDiff) != 0 {
		diffOutput.WriteString("\n### Blame Links:\n")
		githubURL := ghPrClientDetails.GhClientPair.v3Client.BaseURL.String()
		blameUrlPrefix := githubURL + ghPrClientDetails.Owner + "/" + ghPrClientDetails.Repo + "/blame"

		for _, f := range filesWithDiff {
			diffOutput.WriteString("[" + f + "](" + blameUrlPrefix + "/HEAD/" + f + ")\n") // TODO consider switching HEAD to specific SHA
		}
	}

	return hasDiff, diffOutput.String(), nil
}

func CompareRepoDirectories(ghPrClientDetails GhPrClientDetails, sourcePath string, targetPath string, defaultBranch string) (bool, string, error) {
	// Compares two directories content

	// comparing sourcePath targetPath Git object SHA to avoid costly tree compare:
	sourcePathGitObjectSha, err := getDirecotyGitObjectSha(ghPrClientDetails, sourcePath, defaultBranch)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Couldn't get %v, Git object sha: %v", sourcePath, err)
		return false, "", err
	}
	targetPathGitObjectSha, err := getDirecotyGitObjectSha(ghPrClientDetails, targetPath, defaultBranch)
	if err != nil {
		ghPrClientDetails.PrLogger.Errorf("Couldn't get %v, Git object sha: %v", targetPath, err)
		return false, "", err
	}

	if sourcePathGitObjectSha == targetPathGitObjectSha {
		ghPrClientDetails.PrLogger.Debugf("%s(%s) vs %s(%s) git object SHA matched.", sourcePath, sourcePathGitObjectSha, targetPath, targetPathGitObjectSha)
		return false, "", nil
	} else {
		ghPrClientDetails.PrLogger.Debugf("%s(%s) vs %s(%s) git object SHA didn't match! Will do a full tree compare", sourcePath, sourcePathGitObjectSha, targetPath, targetPathGitObjectSha)
		sourceFilesSHAs := make(map[string]string)
		targetFilesSHAs := make(map[string]string)
		hasDiff := false

		generateFlatMapfromFileTree(&ghPrClientDetails, &sourcePath, &sourcePath, &defaultBranch, sourceFilesSHAs)
		generateFlatMapfromFileTree(&ghPrClientDetails, &targetPath, &targetPath, &defaultBranch, targetFilesSHAs)
		// ghPrClientDetails.PrLogger.Infoln(sourceFilesSHAs)
		hasDiff, diffOutput, err := generateDiffOutput(ghPrClientDetails, defaultBranch, sourceFilesSHAs, targetFilesSHAs, sourcePath, targetPath)

		return hasDiff, diffOutput, err
	}
}

func generateFlatMapfromFileTree(ghPrClientDetails *GhPrClientDetails, workingPath *string, rootPath *string, branch *string, listOfFiles map[string]string) {
	getContentOpts := &github.RepositoryContentGetOptions{
		Ref: *branch,
	}
	_, directoryContent, resp, _ := ghPrClientDetails.GhClientPair.v3Client.Repositories.GetContents(ghPrClientDetails.Ctx, ghPrClientDetails.Owner, ghPrClientDetails.Repo, *workingPath, getContentOpts)
	prom.InstrumentGhCall(resp)
	for _, elementInDir := range directoryContent {
		if *elementInDir.Type == "file" {
			relativeName := strings.TrimPrefix(*elementInDir.Path, *rootPath+"/")
			listOfFiles[relativeName] = *elementInDir.SHA
		} else if *elementInDir.Type == "dir" {
			generateFlatMapfromFileTree(ghPrClientDetails, elementInDir.Path, rootPath, branch, listOfFiles)
		} else {
			ghPrClientDetails.PrLogger.Infof("Ignoring type %s for path %s", *elementInDir.Type, *elementInDir.Path)
		}
	}
}
