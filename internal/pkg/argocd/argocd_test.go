package argocd

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/settings"
	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	reposerverApiClient "github.com/argoproj/argo-cd/v2/reposerver/apiclient"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/mocks"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/testutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func readLiveTarget(t *testing.T) (live, target *unstructured.Unstructured, expected string) {
	t.Helper()
	live = readManifest(t, "testdata/"+t.Name()+".live")
	target = readManifest(t, "testdata/"+t.Name()+".target")
	expected = readFileString(t, "testdata/"+t.Name()+".want")
	return live, target, expected
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func readManifest(t *testing.T, path string) *unstructured.Unstructured {
	t.Helper()

	s := readFileString(t, path)
	obj, err := argoappv1.UnmarshalToUnstructured(s)
	if err != nil {
		t.Fatalf("unmarshal %v: %v", path, err)
	}
	return obj
}

func TestDiffLiveVsTargetObject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
	}{
		{"1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			live, target, want := readLiveTarget(t)
			got, err := diffLiveVsTargetObject(live, target)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestRenderDiff(t *testing.T) {
	t.Parallel()
	live := readManifest(t, "testdata/TestRenderDiff.live")
	target := readManifest(t, "testdata/TestRenderDiff.target")
	want := readFileString(t, "testdata/TestRenderDiff.md")
	data, err := diffLiveVsTargetObject(live, target)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// backticks are tricky https://github.com/golang/go/issues/24475
	r := strings.NewReplacer("¬", "`")
	tmpl := r.Replace("¬¬¬diff\n{{.}}¬¬¬\n")

	rendered := renderTemplate(t, tmpl, data)

	if got, want := rendered.String(), want; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func renderTemplate(t *testing.T, tpl string, data any) *bytes.Buffer {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	tmpl := template.New("")
	tmpl = template.Must(tmpl.Parse(tpl))
	if err := tmpl.Execute(buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return buf
}

func TestFindArgocdAppBySHA1Label(t *testing.T) {
	// Here the filtering is done on the ArgoCD server side, so we are just testing the function returns a app
	t.Parallel()
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockApplicationClient := mocks.NewMockApplicationServiceClient(ctrl)
	expectedResponse := &argoappv1.ApplicationList{
		Items: []argoappv1.Application{
			{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"telefonistka.io/component-path-sha1": "111111",
					},
					Name: "right-app",
				},
			},
		},
	}

	mockApplicationClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(expectedResponse, nil)

	app, err := findArgocdAppBySHA1Label(ctx, "random/path", "some-repo", mockApplicationClient)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	if app.Name != "right-app" {
		t.Errorf("App name is not right-app")
	}
}

func TestFindArgocdAppByPathAnnotation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockApplicationClient := mocks.NewMockApplicationServiceClient(ctrl)
	expectedResponse := &argoappv1.ApplicationList{
		Items: []argoappv1.Application{
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"argocd.argoproj.io/manifest-generate-paths": "wrong/path/",
					},
					Name: "wrong-app",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"argocd.argoproj.io/manifest-generate-paths": "right/path/",
					},
					Name: "right-app",
				},
			},
		},
	}

	mockApplicationClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(expectedResponse, nil)

	apps, err := findArgocdAppByManifestPathAnnotation(ctx, "right/path", "some-repo", mockApplicationClient)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	t.Logf("apps: %v", apps)
}

// Here I'm testing a ";" delimted path annotation
func TestFindArgocdAppByPathAnnotationSemiColon(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockApplicationClient := mocks.NewMockApplicationServiceClient(ctrl)
	expectedResponse := &argoappv1.ApplicationList{
		Items: []argoappv1.Application{
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"argocd.argoproj.io/manifest-generate-paths": "wrong/path/;wrong/path2/",
					},
					Name: "wrong-app",
				},
			},
			{ // This is the app we want to find - it has the right path as one of the elements in the annotation
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"argocd.argoproj.io/manifest-generate-paths": "wrong/path/;right/path/",
					},
					Name: "right-app",
				},
			},
		},
	}

	mockApplicationClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(expectedResponse, nil)

	app, err := findArgocdAppByManifestPathAnnotation(ctx, "right/path", "some-repo", mockApplicationClient)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	if app.Name != "right-app" {
		t.Errorf("App name is not right-app")
	}
}

// Here I'm testing a "." path annotation - this is a special case where the path is relative to the repo root specified in the application .spec
func TestFindArgocdAppByPathAnnotationRelative(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockApplicationClient := mocks.NewMockApplicationServiceClient(ctrl)
	expectedResponse := &argoappv1.ApplicationList{
		Items: []argoappv1.Application{
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"argocd.argoproj.io/manifest-generate-paths": ".",
					},
					Name: "right-app",
				},
				Spec: argoappv1.ApplicationSpec{
					Source: &argoappv1.ApplicationSource{
						RepoURL: "",
						Path:    "right/path",
					},
				},
			},
		},
	}

	mockApplicationClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(expectedResponse, nil)
	app, err := findArgocdAppByManifestPathAnnotation(ctx, "right/path", "some-repo", mockApplicationClient)
	if err != nil {
		t.Errorf("Error: %v", err)
	} else if app.Name != "right-app" {
		t.Errorf("App name is not right-app")
	}
}

// Here I'm testing a "." path annotation - this is a special case where the path is relative to the repo root specified in the application .spec
func TestFindArgocdAppByPathAnnotationRelative2(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockApplicationClient := mocks.NewMockApplicationServiceClient(ctrl)
	expectedResponse := &argoappv1.ApplicationList{
		Items: []argoappv1.Application{
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"argocd.argoproj.io/manifest-generate-paths": "./path",
					},
					Name: "right-app",
				},
				Spec: argoappv1.ApplicationSpec{
					Source: &argoappv1.ApplicationSource{
						RepoURL: "",
						Path:    "right/",
					},
				},
			},
		},
	}

	mockApplicationClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(expectedResponse, nil)
	app, err := findArgocdAppByManifestPathAnnotation(ctx, "right/path", "some-repo", mockApplicationClient)
	if err != nil {
		t.Errorf("Error: %v", err)
	} else if app.Name != "right-app" {
		t.Errorf("App name is not right-app")
	}
}

func TestFindArgocdAppByPathAnnotationNotFound(t *testing.T) {
	t.Parallel()
	defer testutils.Quiet()()
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockApplicationClient := mocks.NewMockApplicationServiceClient(ctrl)
	expectedResponse := &argoappv1.ApplicationList{
		Items: []argoappv1.Application{
			{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"argocd.argoproj.io/manifest-generate-paths": "non-existing-path",
					},
					Name: "non-existing-app",
				},
				Spec: argoappv1.ApplicationSpec{
					Source: &argoappv1.ApplicationSource{
						RepoURL: "",
						Path:    "non-existing/",
					},
				},
			},
		},
	}

	mockApplicationClient.EXPECT().List(gomock.Any(), gomock.Any()).Return(expectedResponse, nil)
	app, err := findArgocdAppByManifestPathAnnotation(ctx, "non-existing/path", "some-repo", mockApplicationClient)
	if err != nil {
		t.Errorf("Error: %v", err)
	}
	if app != nil {
		log.Fatal("expected the application to be nil")
	}
}

func TestFetchArgoDiffConcurrently(t *testing.T) {
	t.Parallel()
	// MockApplicationServiceClient
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// mock the argoClients
	mockAppServiceClient := mocks.NewMockApplicationServiceClient(mockCtrl)
	mockSettingsServiceClient := mocks.NewMockSettingsServiceClient(mockCtrl)
	mockProjectServiceClient := mocks.NewMockProjectServiceClient(mockCtrl)
	// fake InitArgoClients

	argoClients := argoCdClients{
		app:     mockAppServiceClient,
		setting: mockSettingsServiceClient,
		project: mockProjectServiceClient,
	}
	// slowReply simulates a slow reply from the server
	slowReply := func(ctx context.Context, in any, opts ...any) {
		time.Sleep(time.Second)
	}

	// makeComponents for test
	makeComponents := func(num int) map[string]bool {
		components := make(map[string]bool, num)
		for i := 0; i < num; i++ {
			components[fmt.Sprintf("component/to/diff/%d", i)] = true
		}
		return components
	}

	mockSettingsServiceClient.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(&settings.Settings{
			URL: "https://test-argocd.test.test",
		}, nil)
	// mock the List method
	mockAppServiceClient.EXPECT().
		List(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&argoappv1.ApplicationList{
			Items: []argoappv1.Application{
				{
					TypeMeta:   metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{},
					Spec:       argoappv1.ApplicationSpec{},
					Status:     argoappv1.ApplicationStatus{},
					Operation:  &argoappv1.Operation{},
				},
			},
		}, nil).
		AnyTimes().
		Do(slowReply) // simulate slow reply

	// mock the Get method
	mockAppServiceClient.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(&argoappv1.Application{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-app",
			},
			Spec: argoappv1.ApplicationSpec{
				Source: &argoappv1.ApplicationSource{
					TargetRevision: "test-revision",
				},
				SyncPolicy: &argoappv1.SyncPolicy{
					Automated: &argoappv1.SyncPolicyAutomated{},
				},
			},
			Status:    argoappv1.ApplicationStatus{},
			Operation: &argoappv1.Operation{},
		}, nil).
		AnyTimes()

	// mock managedResource
	mockAppServiceClient.EXPECT().
		ManagedResources(gomock.Any(), gomock.Any()).
		Return(&application.ManagedResourcesResponse{}, nil).
		AnyTimes()

	// mock the GetManifests method
	mockAppServiceClient.EXPECT().
		GetManifests(gomock.Any(), gomock.Any()).
		Return(&reposerverApiClient.ManifestResponse{}, nil).
		AnyTimes()

	// mock the GetDetailedProject method
	mockProjectServiceClient.EXPECT().
		GetDetailedProject(gomock.Any(), gomock.Any()).
		Return(&project.DetailedProjectsResponse{}, nil).
		AnyTimes()

	const numComponents = 5
	// start timer
	start := time.Now()

	// TODO: Test all the return values, for now we will just ignore the linter.
	_, _, diffResults, _ := GenerateDiffOfChangedComponents( //nolint:dogsled
		context.TODO(),
		makeComponents(numComponents),
		"test-pr-branch",
		"test-repo",
		true,
		false,
		argoClients,
	)

	// stop timer
	elapsed := time.Since(start)
	assert.Equal(t, numComponents, len(diffResults))
	// assert that the entire run takes less than numComponents * 1 second
	assert.Less(t, elapsed, time.Duration(numComponents)*time.Second)
}
