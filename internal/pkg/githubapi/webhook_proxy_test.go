package githubapi

// @Title
// @Description
// @Author
// @Update
import (
	"sort"
	"testing"

	"github.com/go-test/deep"
	"github.com/google/go-github/v62/github"
	cfg "github.com/wayfair-incubator/telefonistka/internal/pkg/configuration"
)

func TestGenerateListOfEndpoints(t *testing.T) {
	t.Parallel()
	config := &cfg.Config{
		WebhookEndpointRegexs: []cfg.WebhookEndpointRegex{
			{
				Expression: `^workspace\/[^/]*\/.*`,
				Replacements: []string{
					`https://blabla.com/webhook`,
				},
			},
			{
				Expression: `^clusters\/([^/]*)\/([^/]*)\/([^/]*)\/.*`,
				Replacements: []string{
					`https://ingress-a-${1}-${2}-${3}.example.com/webhook`,
					`https://ingress-b-${1}-${2}-${3}.example.com/webhook`,
				},
			},
		},
	}
	listOfFiles := []string{
		"workspace/csi-verify/values/global.yaml",
		"clusters/sdeprod/dsm1/c1/csi-verify/values/global.yaml",
	}

	endpoints := generateListOfEndpoints(listOfFiles, config)
	expectedEndpoints := []string{
		"https://blabla.com/webhook",
		"https://ingress-a-sdeprod-dsm1-c1.example.com/webhook",
		"https://ingress-b-sdeprod-dsm1-c1.example.com/webhook",
	}

	sort.Strings(endpoints)
	sort.Strings(expectedEndpoints)
	if diff := deep.Equal(endpoints, expectedEndpoints); diff != nil {
		t.Error(diff)
	}
}

func TestGenerateListOfChangedFiles(t *testing.T) {
	t.Parallel()
	eventPayload := &github.PushEvent{
		Commits: []*github.HeadCommit{
			{
				Added: []string{
					"workspace/csi-verify/values/global-new.yaml",
				},
				Removed: []string{
					"workspace/csi-verify/values/global-old.yaml",
				},
				SHA: github.String("000001"),
			},
			{
				Modified: []string{
					"clusters/sdeprod/dsm1/c1/csi-verify/values/global.yaml",
				},
				SHA: github.String("000002"),
			},
		},
	}

	listOfFiles := generateListOfChangedFiles(eventPayload)
	expectedListOfFiles := []string{
		"workspace/csi-verify/values/global-new.yaml",
		"workspace/csi-verify/values/global-old.yaml",
		"clusters/sdeprod/dsm1/c1/csi-verify/values/global.yaml",
	}

	sort.Strings(listOfFiles)
	sort.Strings(expectedListOfFiles)

	if diff := deep.Equal(listOfFiles, expectedListOfFiles); diff != nil {
		t.Error(diff)
	}
}
