package argocd

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: Blocklisted import crypto/sha1: weak cryptographic primitive (gosec), this is not a cryptographic use case
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cmdutil "github.com/argoproj/argo-cd/v2/cmd/util"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/settings"
	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	argodiff "github.com/argoproj/argo-cd/v2/util/argo/diff"
	"github.com/argoproj/argo-cd/v2/util/argo/normalizers"
	argoio "github.com/argoproj/argo-cd/v2/util/io"
	"github.com/argoproj/gitops-engine/pkg/sync/hook"
	"github.com/google/go-cmp/cmp"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DiffElement struct to store diff element details, this represents a single k8s object
type DiffElement struct {
	ObjectGroup     string
	ObjectName      string
	ObjectKind      string
	ObjectNamespace string
	Diff            string
}

// DiffResult struct to store diff result
type DiffResult struct {
	ComponentPath string
	ArgoCdAppName string
	ArgoCdAppURL  string
	DiffElements  []DiffElement
	HasDiff       bool
	DiffError     error
}

// Mostly copied from  https://github.com/argoproj/argo-cd/blob/4f6a8dce80f0accef7ed3b5510e178a6b398b331/cmd/argocd/commands/app.go#L1255C6-L1338
// But instead of printing the diff to stdout, we return it as a string in a struct so we can format it in a nice PR comment.
func generateArgocdAppDiff(ctx context.Context, app *argoappv1.Application, proj *argoappv1.AppProject, resources *application.ManagedResourcesResponse, argoSettings *settings.Settings, diffOptions *DifferenceOption) (foundDiffs bool, diffElements []DiffElement, err error) {
	liveObjs, err := cmdutil.LiveObjects(resources.Items)
	if err != nil {
		return false, nil, fmt.Errorf("Failed to get live objects: %v", err)
	}

	items := make([]objKeyLiveTarget, 0)
	var unstructureds []*unstructured.Unstructured
	for _, mfst := range diffOptions.res.Manifests {
		obj, err := argoappv1.UnmarshalToUnstructured(mfst)
		if err != nil {
			return false, nil, fmt.Errorf("Failed to unmarshal manifest: %v", err)
		}
		unstructureds = append(unstructureds, obj)
	}
	groupedObjs, err := groupObjsByKey(unstructureds, liveObjs, app.Spec.Destination.Namespace)
	if err != nil {
		return false, nil, fmt.Errorf("Failed to group objects by key: %v", err)
	}
	items, err = groupObjsForDiff(resources, groupedObjs, items, argoSettings, app.InstanceName(argoSettings.ControllerNamespace), app.Spec.Destination.Namespace)
	if err != nil {
		return false, nil, fmt.Errorf("Failed to group objects for diff: %v", err)
	}

	for _, item := range items {
		var diffElement DiffElement
		if item.target != nil && hook.IsHook(item.target) || item.live != nil && hook.IsHook(item.live) {
			continue
		}
		overrides := make(map[string]argoappv1.ResourceOverride)
		for k := range argoSettings.ResourceOverrides {
			val := argoSettings.ResourceOverrides[k]
			overrides[k] = *val
		}

		ignoreAggregatedRoles := false
		ignoreNormalizerOpts := normalizers.IgnoreNormalizerOpts{}
		diffConfig, err := argodiff.NewDiffConfigBuilder().
			WithDiffSettings(app.Spec.IgnoreDifferences, overrides, ignoreAggregatedRoles, ignoreNormalizerOpts).
			WithTracking(argoSettings.AppLabelKey, argoSettings.TrackingMethod).
			WithNoCache().
			Build()
		if err != nil {
			return false, nil, fmt.Errorf("Failed to build diff config: %v", err)
		}
		diffRes, err := argodiff.StateDiff(item.live, item.target, diffConfig)
		if err != nil {
			return false, nil, fmt.Errorf("Failed to diff objects: %v", err)
		}

		if diffRes.Modified || item.target == nil || item.live == nil {
			diffElement.ObjectGroup = item.key.Group
			diffElement.ObjectKind = item.key.Kind
			diffElement.ObjectNamespace = item.key.Namespace
			diffElement.ObjectName = item.key.Name

			var live *unstructured.Unstructured
			var target *unstructured.Unstructured
			if item.target != nil && item.live != nil {
				target = &unstructured.Unstructured{}
				live = item.live
				err = json.Unmarshal(diffRes.PredictedLive, target)
				if err != nil {
					return false, nil, fmt.Errorf("Failed to unmarshal predicted live object: %v", err)
				}
			} else {
				live = item.live
				target = item.target
			}
			if !foundDiffs {
				foundDiffs = true
			}

			diffElement.Diff, err = diffLiveVsTargetObject(live, target)
			if err != nil {
				return false, nil, fmt.Errorf("Failed to diff live objects: %v", err)
			}
		}
		diffElements = append(diffElements, diffElement)
	}
	return foundDiffs, diffElements, nil
}

// Should return output that is compatible with github markdown diff highlighting format
func diffLiveVsTargetObject(live, target *unstructured.Unstructured) (string, error) {
	patch := cmp.Diff(live, target)
	return patch, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func createArgoCdClient() (apiclient.Client, error) {
	plaintext, _ := strconv.ParseBool(getEnv("ARGOCD_PLAINTEXT", "false"))
	insecure, _ := strconv.ParseBool(getEnv("ARGOCD_INSECURE", "false"))

	opts := &apiclient.ClientOptions{
		ServerAddr: getEnv("ARGOCD_SERVER_ADDR", "localhost:8080"),
		AuthToken:  getEnv("ARGOCD_TOKEN", ""),
		PlainText:  plaintext,
		Insecure:   insecure,
	}

	clientset, err := apiclient.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("Error creating ArgoCD API client: %v", err)
	}
	return clientset, nil
}

// findArgocdAppBySHA1Label finds an ArgoCD application by the SHA1 label of the component path it's supposed to avoid performance issues with the "manifest-generate-paths" annotation method which requires pulling all ArgoCD applications(!) on every PR event.
// The SHA1 label is assumed to be populated by the ApplicationSet controller(or apps of apps  or similar).
func findArgocdAppBySHA1Label(ctx context.Context, componentPath string, repo string, appClient application.ApplicationServiceClient) (app *argoappv1.Application, err error) {
	// Calculate sha1 of component path to use in a label selector
	cPathBa := []byte(componentPath)
	hasher := sha1.New() //nolint:gosec // G505: Blocklisted import crypto/sha1: weak cryptographic primitive (gosec), this is not a cryptographic use case
	hasher.Write(cPathBa)
	componentPathSha1 := hex.EncodeToString(hasher.Sum(nil))
	labelSelector := fmt.Sprintf("telefonistka.io/component-path-sha1=%s", componentPathSha1)
	log.Debugf("Using label selector: %s", labelSelector)
	appLabelQuery := application.ApplicationQuery{
		Selector: &labelSelector,
		Repo:     &repo,
	}
	foundApps, err := appClient.List(ctx, &appLabelQuery)
	if err != nil {
		return nil, fmt.Errorf("Error listing ArgoCD applications: %v", err)
	}
	if len(foundApps.Items) == 0 {
		return nil, fmt.Errorf("No ArgoCD application found for component path sha1 %s(repo %s), used this label selector: %s", componentPathSha1, repo, labelSelector)
	}

	// we expect only one app with this label and repo selectors
	return &foundApps.Items[0], nil
}

// findArgocdAppByManifestPathAnnotation is the default method to find an ArgoCD application by the manifest-generate-paths annotation.
// It assumes the ArgoCD (optional) manifest-generate-paths annotation is set on all relevant apps.
// Notice that this method includes a full list of all ArgoCD applications in the repo, this could be a performance issue if there are many apps in the repo.
func findArgocdAppByManifestPathAnnotation(ctx context.Context, componentPath string, repo string, appClient application.ApplicationServiceClient) (app *argoappv1.Application, err error) {
	// argocd.argoproj.io/manifest-generate-paths
	appQuery := application.ApplicationQuery{
		Repo: &repo,
	}
	// AFAIKT I can't use standard grpc instrumentation here, since the argocd client abstracts too much (including the choice between Grpc and Grpc-web)
	// I'll just manually log the time it takes to get the apps for now
	getAppsStart := time.Now()
	allRepoApps, err := appClient.List(ctx, &appQuery)
	getAppsDuration := time.Since(getAppsStart).Milliseconds()
	log.Infof("Got %v ArgoCD applications for repo %s in %v ms", len(allRepoApps.Items), repo, getAppsDuration)
	if err != nil {
		return nil, err
	}
	for _, app := range allRepoApps.Items {
		// Check if the app has the annotation
		// https://argo-cd.readthedocs.io/en/stable/operator-manual/high_availability/#manifest-paths-annotation
		// Consider the annotation content can a semi-colon separated list of paths, an absolute path or a relative path(start with a ".")  and the manifest-paths-annotation could be a subpath of componentPath.
		// We need to check if the annotation is a subpath of componentPath

		appManifestPathsAnnotation := app.Annotations["argocd.argoproj.io/manifest-generate-paths"]

		for _, manifetsPathElement := range strings.Split(appManifestPathsAnnotation, ";") {
			// if `manifest-generate-paths` element starts with a "." it is a relative path(relative to repo root), we need to join it with the app source path
			if strings.HasPrefix(manifetsPathElement, ".") {
				manifetsPathElement = filepath.Join(app.Spec.Source.Path, manifetsPathElement)
			}

			// Checking is componentPath is a subpath of the manifetsPathElement
			// Using filepath.Rel solves all kinds of path issues, like double slashes, etc.
			rel, err := filepath.Rel(manifetsPathElement, componentPath)
			if !strings.HasPrefix(rel, "..") && err == nil {
				log.Debugf("Found app %s with manifest-generate-paths(\"%s\") annotation that matches %s", app.Name, appManifestPathsAnnotation, componentPath)
				return &app, nil
			}
		}
	}
	return nil, fmt.Errorf("No ArgoCD application found with manifest-generate-paths annotation that matches %s(looked at repo %s, checked %v apps)	", componentPath, repo, len(allRepoApps.Items))
}

func generateDiffOfAComponent(ctx context.Context, componentPath string, prBranch string, repo string, appClient application.ApplicationServiceClient, projClient projectpkg.ProjectServiceClient, argoSettings *settings.Settings, useSHALabelForArgoDicovery bool) (componentDiffResult DiffResult) {
	componentDiffResult.ComponentPath = componentPath

	// Find ArgoCD application by the path SHA1 label selector and repo name
	// At the moment we assume one to one mapping between Telefonistka components and ArgoCD application

	var foundApp *argoappv1.Application
	var err error
	if useSHALabelForArgoDicovery {
		foundApp, err = findArgocdAppBySHA1Label(ctx, componentPath, repo, appClient)
	} else {
		foundApp, err = findArgocdAppByManifestPathAnnotation(ctx, componentPath, repo, appClient)
	}
	if err != nil {
		componentDiffResult.DiffError = err
		return componentDiffResult
	}

	log.Debugf("Found ArgoCD application: %s", foundApp.Name)
	// Get the application and its resources, resources are the live state of the application objects.
	// The 2nd "app fetch" is needed for the "refreshTypeHArd", we don't want to do that to non-relevant apps"
	refreshType := string(argoappv1.RefreshTypeHard)
	appNameQuery := application.ApplicationQuery{
		Name:    &foundApp.Name, // we expect only one app with this label and repo selectors
		Refresh: &refreshType,
	}
	app, err := appClient.Get(ctx, &appNameQuery)
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting app %s: %v", foundApp.Name, err)
		return componentDiffResult
	}
	log.Debugf("Got ArgoCD app %s", app.Name)
	componentDiffResult.ArgoCdAppName = app.Name
	componentDiffResult.ArgoCdAppURL = fmt.Sprintf("%s/applications/%s", argoSettings.URL, app.Name)
	resources, err := appClient.ManagedResources(ctx, &application.ResourcesQuery{ApplicationName: &app.Name, AppNamespace: &app.Namespace})
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting (live)resources for app %s: %v", app.Name, err)
		return componentDiffResult
	}
	log.Debugf("Got (live)resources for app %s", app.Name)

	// Get the application manifests, these are the target state of the application objects, taken from the git repo, specificly from the PR branch.
	diffOption := &DifferenceOption{}

	manifestQuery := application.ApplicationManifestQuery{
		Name:         &app.Name,
		Revision:     &prBranch,
		AppNamespace: &app.Namespace,
	}
	manifests, err := appClient.GetManifests(ctx, &manifestQuery)
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting manifests for app %s, revision %s: %v", app.Name, prBranch, err)
		return componentDiffResult
	}
	log.Debugf("Got manifests for app %s, revision %s", app.Name, prBranch)
	diffOption.res = manifests
	diffOption.revision = prBranch

	// Now we diff the live state(resources) and target state of the application objects(diffOption.res)
	detailedProject, err := projClient.GetDetailedProject(ctx, &projectpkg.ProjectQuery{Name: app.Spec.Project})
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting project %s: %v", app.Spec.Project, err)
		return componentDiffResult
	}

	componentDiffResult.HasDiff, componentDiffResult.DiffElements, err = generateArgocdAppDiff(ctx, app, detailedProject.Project, resources, argoSettings, diffOption)
	if err != nil {
		componentDiffResult.DiffError = err
	}

	return componentDiffResult
}

// GenerateDiffOfChangedComponents generates diff of changed components
func GenerateDiffOfChangedComponents(ctx context.Context, componentPathList []string, prBranch string, repo string, useSHALabelForArgoDicovery bool) (hasComponentDiff bool, hasComponentDiffErrors bool, diffResults []DiffResult, err error) {
	hasComponentDiff = false
	hasComponentDiffErrors = false
	// env var should be centralized
	client, err := createArgoCdClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD client: %v", err)
		return false, true, nil, err
	}

	conn, appClient, err := client.NewApplicationClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD app client: %v", err)
		return false, true, nil, err
	}
	defer argoio.Close(conn)

	conn, projClient, err := client.NewProjectClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD project client: %v", err)
		return false, true, nil, err
	}
	defer argoio.Close(conn)

	conn, settingClient, err := client.NewSettingsClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD settings client: %v", err)
		return false, true, nil, err
	}
	defer argoio.Close(conn)
	argoSettings, err := settingClient.Get(ctx, &settings.SettingsQuery{})
	if err != nil {
		log.Errorf("Error getting ArgoCD settings: %v", err)
		return false, true, nil, err
	}

	log.Debugf("Checking ArgoCD diff for components: %v", componentPathList)
	for _, componentPath := range componentPathList {
		currentDiffResult := generateDiffOfAComponent(ctx, componentPath, prBranch, repo, appClient, projClient, argoSettings, useSHALabelForArgoDicovery)
		if currentDiffResult.DiffError != nil {
			log.Errorf("Error generating diff for component %s: %v", componentPath, currentDiffResult.DiffError)
			hasComponentDiffErrors = true
			err = currentDiffResult.DiffError
		}
		if currentDiffResult.HasDiff {
			hasComponentDiff = true
		}
		diffResults = append(diffResults, currentDiffResult)
	}

	return hasComponentDiff, hasComponentDiffErrors, diffResults, err
}
