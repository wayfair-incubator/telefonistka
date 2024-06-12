package argocd

import (
	"context"
	"testing"

	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/golang/mock/gomock"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/mocks"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
