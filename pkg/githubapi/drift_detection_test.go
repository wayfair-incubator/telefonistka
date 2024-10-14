package githubapi

import (
	"context"
	"testing"

	"github.com/go-test/deep"
	"github.com/google/go-github/v62/github"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	log "github.com/sirupsen/logrus"
)

func TestGenerateFlatMapfromFileTree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	filesSHAs := make(map[string]string)

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposContentsByOwnerByRepoByPath,
			[]github.RepositoryContent{
				{
					Type: github.String("file"),
					Path: github.String("some/path/file1"),
					SHA:  github.String("fffff1"),
				},
				{
					Type: github.String("file"),
					Path: github.String("some/path/file2"),
					SHA:  github.String("fffff2"),
				},
				{
					Type: github.String("dir"),
					Path: github.String("some/path/dir1"),
					SHA:  github.String("fffff3"),
				},
			},
			[]github.RepositoryContent{
				{
					Type: github.String("file"),
					Path: github.String("some/path/dir1/file4"),
					SHA:  github.String("fffff4"),
				},
				{
					Type: github.String("dir"),
					Path: github.String("some/path/dir1/nested_dir1/"),
					SHA:  github.String("fffff3"),
				},
			},
			[]github.RepositoryContent{
				{
					Type: github.String("file"),
					Path: github.String("some/path/dir1/nested_dir1/file5"),
					SHA:  github.String("fffff5"),
				},
			},
		),
	)
	ghClientPair := GhClientPair{v3Client: github.NewClient(mockedHTTPClient)}

	ghPrClientDetails := GhPrClientDetails{
		Ctx:          ctx,
		GhClientPair: &ghClientPair,
		Owner:        "AnOwner",
		Repo:         "Arepo",
		PrNumber:     120,
		Ref:          "Abranch",
		PrLogger: log.WithFields(log.Fields{
			"repo":     "AnOwner/Arepo",
			"prNumber": 120,
		}),
	}
	expectedFilesSHAs := map[string]string{
		"file1":                  "fffff1",
		"file2":                  "fffff2",
		"dir1/file4":             "fffff4",
		"dir1/nested_dir1/file5": "fffff5",
	}

	defaultBranch := "main"
	targetPath := "some/path"
	generateFlatMapfromFileTree(&ghPrClientDetails, &targetPath, &targetPath, &defaultBranch, filesSHAs)
	if diff := deep.Equal(expectedFilesSHAs, filesSHAs); diff != nil {
		for _, l := range diff {
			t.Error(l)
		}
	}
}

func TestGenerateDiffOutputDiffFileContent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposContentsByOwnerByRepoByPath,
			github.RepositoryContent{
				Content: github.String("File A content\n"),
			},
			github.RepositoryContent{
				Content: github.String("File B content\n"),
			},
		),
	)

	ghClientPair := GhClientPair{v3Client: github.NewClient(mockedHTTPClient)}

	ghPrClientDetails := GhPrClientDetails{
		Ctx:          ctx,
		GhClientPair: &ghClientPair,
		Owner:        "AnOwner",
		Repo:         "Arepo",
		PrNumber:     120,
		Ref:          "Abranch",
		PrLogger: log.WithFields(log.Fields{
			"repo":     "AnOwner/Arepo",
			"prNumber": 120,
		}),
	}

	// TODO move this to file
	expectedDiffOutput := "\n```" + `diff
--- source-path/file-1.text
+++ target-path/file-1.text
@@ -1 +1 @@
-File A content
+File B content` + "\n\n```\n\n" + `### Blame Links:
[source-path/file-1.text](https://api.github.com/AnOwner/Arepo/blame/HEAD/source-path/file-1.text)
`

	var sourceFilesSHAs map[string]string
	var targetFilesSHAs map[string]string

	sourceFilesSHAs = make(map[string]string)
	targetFilesSHAs = make(map[string]string)

	sourceFilesSHAs["file-1.text"] = "000001"
	targetFilesSHAs["file-1.text"] = "000002"

	isDiff, diffOutput, err := generateDiffOutput(ghPrClientDetails, "main", sourceFilesSHAs, targetFilesSHAs, "source-path", "target-path")
	if err != nil {
		t.Fatalf("generating diff output failed: err=%s", err)
	}

	if diffOutput != expectedDiffOutput {
		edits := myers.ComputeEdits(span.URIFromPath("diff.text"), diffOutput, expectedDiffOutput)
		t.Fatalf("Diff Output is wrong:\n%s", gotextdiff.ToUnified("computed", "expected", diffOutput, edits))
	}
	if !isDiff {
		t.Fatal("Did not detect diff in in files with different SHAs/content")
	}
}

func TestGenerateDiffOutputIdenticalFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mockedHTTPClient := mock.NewMockedHTTPClient()

	ghClientPair := GhClientPair{v3Client: github.NewClient(mockedHTTPClient)}

	ghPrClientDetails := GhPrClientDetails{
		Ctx:          ctx,
		GhClientPair: &ghClientPair,
		Owner:        "AnOwner",
		Repo:         "Arepo",
		PrNumber:     120,
		Ref:          "Abranch",
		PrLogger: log.WithFields(log.Fields{
			"repo":     "AnOwner/Arepo",
			"prNumber": 120,
		}),
	}

	var sourceFilesSHAs map[string]string
	var targetFilesSHAs map[string]string

	sourceFilesSHAs = make(map[string]string)
	targetFilesSHAs = make(map[string]string)

	sourceFilesSHAs["file-1.text"] = "000001"
	targetFilesSHAs["file-1.text"] = "000001"

	isDiff, _, err := generateDiffOutput(ghPrClientDetails, "main", sourceFilesSHAs, targetFilesSHAs, "source-path", "target-path")
	if err != nil {
		t.Fatalf("generating diff output failed: err=%s", err)
	}

	if isDiff {
		t.Error("Found a Diff in files with identical SHA/content")
	}
}

func TestGenerateDiffOutputMissingSourceFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposContentsByOwnerByRepoByPath,
			github.RepositoryContent{
				Content: github.String("File A content\n"),
			},
		),
	)

	ghClientPair := GhClientPair{v3Client: github.NewClient(mockedHTTPClient)}

	ghPrClientDetails := GhPrClientDetails{
		Ctx:          ctx,
		GhClientPair: &ghClientPair,
		Owner:        "AnOwner",
		Repo:         "Arepo",
		PrNumber:     120,
		Ref:          "Abranch",
		PrLogger: log.WithFields(log.Fields{
			"repo":     "AnOwner/Arepo",
			"prNumber": 120,
		}),
	}

	expectedDiffOutput := "\n```" + `diff
+++ target-path/file-1.text (missing from source dir source-path)` + "\n\n```\n"

	var sourceFilesSHAs map[string]string
	var targetFilesSHAs map[string]string

	sourceFilesSHAs = make(map[string]string)
	targetFilesSHAs = make(map[string]string)

	targetFilesSHAs["file-1.text"] = "000001"

	isDiff, diffOutput, err := generateDiffOutput(ghPrClientDetails, "main", sourceFilesSHAs, targetFilesSHAs, "source-path", "target-path")
	if err != nil {
		t.Fatalf("generating diff output failed: err=%s", err)
	}

	if diffOutput != expectedDiffOutput {
		t.Errorf("Diff Output is wrong:\n%s\n vs:\n%s\n", diffOutput, expectedDiffOutput)
	}
	if !isDiff {
		t.Errorf("Did not detect diff in in files with different SHAs/content, isDiff=%t", isDiff)
	}
}

func TestGenerateDiffOutputMissingTargetFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	mockedHTTPClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatch(
			mock.GetReposContentsByOwnerByRepoByPath,
			github.RepositoryContent{
				Content: github.String("File A content\n"),
			},
		),
	)

	ghClientPair := GhClientPair{v3Client: github.NewClient(mockedHTTPClient)}

	ghPrClientDetails := GhPrClientDetails{
		Ctx:          ctx,
		GhClientPair: &ghClientPair,
		Owner:        "AnOwner",
		Repo:         "Arepo",
		PrNumber:     120,
		Ref:          "Abranch",
		PrLogger: log.WithFields(log.Fields{
			"repo":     "AnOwner/Arepo",
			"prNumber": 120,
		}),
	}

	expectedDiffOutput := "\n```" + `diff
--- source-path/file-1.text (missing from target dir target-path)` + "\n\n```\n"

	var sourceFilesSHAs map[string]string
	var targetFilesSHAs map[string]string

	sourceFilesSHAs = make(map[string]string)
	targetFilesSHAs = make(map[string]string)

	sourceFilesSHAs["file-1.text"] = "000001"

	isDiff, diffOutput, err := generateDiffOutput(ghPrClientDetails, "main", sourceFilesSHAs, targetFilesSHAs, "source-path", "target-path")
	if err != nil {
		t.Fatalf("generating diff output failed: err=%s", err)
	}

	if diffOutput != expectedDiffOutput {
		t.Errorf("Diff Output is wrong(computed):\n%s\n vs(expected):\n%s\n", diffOutput, expectedDiffOutput)
	}
	if !isDiff {
		t.Errorf("Did not detect diff in in files with different SHAs/content, isDiff=%t", isDiff)
	}
}
