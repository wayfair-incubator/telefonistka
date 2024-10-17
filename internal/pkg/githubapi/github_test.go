package githubapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSafePromotionBranchName(t *testing.T) {
	t.Parallel()
	prNumber := 11
	originBranch := "originBranch"
	targetPaths := []string{"targetPath1", "targetPath2"}
	result := generateSafePromotionBranchName(prNumber, originBranch, targetPaths)
	expectedResult := "promotions/11-originBranch-676f02019f18"
	if result != expectedResult {
		t.Errorf("Expected %s, got %s", expectedResult, result)
	}
}

// TestGenerateSafePromotionBranchNameLongBranchName tests the case where the original  branch name is longer than 250 characters
func TestGenerateSafePromotionBranchNameLongBranchName(t *testing.T) {
	t.Parallel()
	prNumber := 11

	originBranch := string(bytes.Repeat([]byte("originBranch"), 100))
	targetPaths := []string{"targetPath1", "targetPath2"}
	result := generateSafePromotionBranchName(prNumber, originBranch, targetPaths)
	if len(result) > 250 {
		t.Errorf("Expected branch name to be less than 250 characters, got %d", len(result))
	}
}

// TestGenerateSafePromotionBranchNameLongTargets tests the case where the target paths are longer than 250 characters
func TestGenerateSafePromotionBranchNameLongTargets(t *testing.T) {
	t.Parallel()
	prNumber := 11
	originBranch := "originBranch"
	targetPaths := []string{
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/1",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/2",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/3",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/4",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/5",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/6",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/7",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/8",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/9",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/10",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/11",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/12",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/13",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/14",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/15",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/16",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/17",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/18",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/19",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong/target/path/20",
	}
	result := generateSafePromotionBranchName(prNumber, originBranch, targetPaths)
	if len(result) > 250 {
		t.Errorf("Expected branch name to be less than 250 characters, got %d", len(result))
	}
}

func TestAnalyzeCommentUpdateCheckBox(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		newBody                  string
		oldBody                  string
		checkboxIdentifier       string
		expectedWasCheckedBefore bool
		expectedIsCheckedNow     bool
	}{
		"Checkbox is marked": {
			oldBody: `This is a comment
foobar
- [ ] <!-- check-slug-1 --> Description of checkbox
foobar`,
			newBody: `This is a comment
foobar
- [x] <!-- check-slug-1 --> Description of checkbox
foobar`,
			checkboxIdentifier:       "check-slug-1",
			expectedWasCheckedBefore: false,
			expectedIsCheckedNow:     true,
		},
		"Checkbox is unmarked": {
			oldBody: `This is a comment
foobar
- [x] <!-- check-slug-1 --> Description of checkbox
foobar`,
			newBody: `This is a comment
foobar
- [ ] <!-- check-slug-1 --> Description of checkbox
foobar`,
			checkboxIdentifier:       "check-slug-1",
			expectedWasCheckedBefore: true,
			expectedIsCheckedNow:     false,
		},
		"Checkbox isn't in comment body": {
			oldBody: `This is a comment
foobar
foobar`,
			newBody: `This is a comment
foobar
foobar`,
			checkboxIdentifier:       "check-slug-1",
			expectedWasCheckedBefore: false,
			expectedIsCheckedNow:     false,
		},
	}
	for name, tc := range tests {
		tc := tc // capture range variable
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			wasCheckedBefore, isCheckedNow := analyzeCommentUpdateCheckBox(tc.newBody, tc.oldBody, tc.checkboxIdentifier)
			if isCheckedNow != tc.expectedIsCheckedNow {
				t.Errorf("%s: Expected isCheckedNow to be %v, got %v", name, tc.expectedIsCheckedNow, isCheckedNow)
			}
			if wasCheckedBefore != tc.expectedWasCheckedBefore {
				t.Errorf("%s: Expected wasCheckedBeforeto to be %v, got %v", name, tc.expectedWasCheckedBefore, wasCheckedBefore)
			}
		})
	}
}

func TestIsSyncFromBranchAllowedForThisPath(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		allowedPathRegex string
		path             string
		expectedResult   bool
	}{
		"Path is allowed": {
			allowedPathRegex: `^workspace/.*$`,
			path:             "workspace/app3",
			expectedResult:   true,
		},
		"Path is not allowed": {
			allowedPathRegex: `^workspace/.*$`,
			path:             "clusters/prod/aws/eu-east-1/app3",
			expectedResult:   false,
		},
	}

	for name, tc := range tests {
		tc := tc // capture range variable
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := isSyncFromBranchAllowedForThisPath(tc.allowedPathRegex, tc.path)
			if result != tc.expectedResult {
				t.Errorf("%s: Expected result to be %v, got %v", name, tc.expectedResult, result)
			}
		})
	}
}

func TestGenerateArgoCdDiffComments(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		diffCommentDataTestDataFileName string
		expectedNumberOfComments        int
		maxCommentLength                int
	}{
		"All cluster diffs fit in one comment": {
			diffCommentDataTestDataFileName: "./testdata/diff_comment_data_test.json",
			expectedNumberOfComments:        1,
			maxCommentLength:                65535,
		},
		"Split diffs, one cluster per comment": {
			diffCommentDataTestDataFileName: "./testdata/diff_comment_data_test.json",
			expectedNumberOfComments:        3,
			maxCommentLength:                1000,
		},
		"Split diffs, but maxCommentLength is very small so need to use the concise template": {
			diffCommentDataTestDataFileName: "./testdata/diff_comment_data_test.json",
			expectedNumberOfComments:        3,
			maxCommentLength:                600,
		},
	}

	for name, tc := range tests {
		tc := tc // capture range variable
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var diffCommentData DiffCommentData
			readJSONFromFile(t, tc.diffCommentDataTestDataFileName, &diffCommentData)

			result, err := generateArgoCdDiffComments(diffCommentData, tc.maxCommentLength)
			if err != nil {
				t.Errorf("Error generating diff comments: %s", err)
			}
			if len(result) != tc.expectedNumberOfComments {
				t.Errorf("%s: Expected number of comments to be %v, got %v", name, tc.expectedNumberOfComments, len(result))
			}
			for _, comment := range result {
				if len(comment) > tc.maxCommentLength {
					t.Errorf("%s: Expected comment length to be less than %d, got %d", name, tc.maxCommentLength, len(comment))
				}
			}
		})
	}
}

func readJSONFromFile(t *testing.T, filename string, data interface{}) {
	t.Helper()
	// Read the JSON from the file
	jsonData, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Error loading test data file: %s", err)
	}

	// Unserialize the JSON into the provided struct
	err = json.Unmarshal(jsonData, data)
	if err != nil {
		t.Fatalf("Error unmarshalling JSON: %s", err)
	}
}

func TestPrBody(t *testing.T) {
	t.Parallel()
	keys := []int{1, 2, 3}
	newPrMetadata := prMetadata{
		PreviousPromotionMetadata: map[int]promotionInstanceMetaData{
			1: {
				SourcePath:  "sourcePath1",
				TargetPaths: []string{"targetPath1", "targetPath2"},
			},
			2: {
				SourcePath:  "sourcePath2",
				TargetPaths: []string{"targetPath3", "targetPath4"},
			},
			3: {
				SourcePath:  "sourcePath3",
				TargetPaths: []string{"targetPath5", "targetPath6"},
			},
		},
	}
	newPrBody := prBody(keys, newPrMetadata, "")
	expectedPrBody, err := os.ReadFile("testdata/pr_body.golden.md")
	if err != nil {
		t.Fatalf("Error loading golden file: %s", err)
	}
	assert.Equal(t, string(expectedPrBody), newPrBody)
}

func TestGhPrClientDetailsGetBlameURLPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Host      string
		Owner     string
		Repo      string
		ExpectURL string
	}{
		{
			"",
			"commercetools",
			"test",
			fmt.Sprintf("%s/commercetools/test/blame", githubPublicBaseURL),
		},
		{
			"https://myserver.github.com",
			"some-other-owner",
			"some-other-repo",
			"https://myserver.github.com/some-other-owner/some-other-repo/blame",
		},
	}

	// reset the GITHUB_HOST env to prevent conflicts with other tests.
	defer os.Unsetenv("GITHUB_HOST")

	for _, tc := range tests {
		os.Setenv("GITHUB_HOST", tc.Host)
		ghPrClientDetails := &GhPrClientDetails{Owner: tc.Owner, Repo: tc.Repo}
		blameURLPrefix := ghPrClientDetails.getBlameURLPrefix()
		assert.Equal(t, tc.ExpectURL, blameURLPrefix)
	}
}
