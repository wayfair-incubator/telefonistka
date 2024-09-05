package argocd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"text/template"

	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/golang/mock/gomock"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/mocks"
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
