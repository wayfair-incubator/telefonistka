package argocd

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: Blocklisted import crypto/sha1: weak cryptographic primitive (gosec), this is not a cryptographic use case
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/argoproj/argo-cd/v2/applicationset/utils"
	cmdutil "github.com/argoproj/argo-cd/v2/cmd/util"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	applicationsetpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/applicationset"
	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/settings"
	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	argodiff "github.com/argoproj/argo-cd/v2/util/argo/diff"
	"github.com/argoproj/argo-cd/v2/util/argo/normalizers"
	"github.com/argoproj/gitops-engine/pkg/sync/hook"
	log "github.com/sirupsen/logrus"
	"github.com/wayfair-incubator/telefonistka/internal/pkg/argocd/diff"
	yaml2 "gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ctxLines is the number of context lines used in application diffs.
const ctxLines = 10

type argoCdClients struct {
	app     application.ApplicationServiceClient
	project projectpkg.ProjectServiceClient
	setting settings.SettingsServiceClient
	appSet  applicationsetpkg.ApplicationSetServiceClient
}

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
	ComponentPath            string
	ArgoCdAppName            string
	ArgoCdAppURL             string
	DiffElements             []DiffElement
	HasDiff                  bool
	DiffError                error
	AppWasTemporarilyCreated bool
}

// Mostly copied from  https://github.com/argoproj/argo-cd/blob/4f6a8dce80f0accef7ed3b5510e178a6b398b331/cmd/argocd/commands/app.go#L1255C6-L1338
// But instead of printing the diff to stdout, we return it as a string in a struct so we can format it in a nice PR comment.
func generateArgocdAppDiff(ctx context.Context, keepDiffData bool, app *argoappv1.Application, proj *argoappv1.AppProject, resources *application.ManagedResourcesResponse, argoSettings *settings.Settings, diffOptions *DifferenceOption) (foundDiffs bool, diffElements []DiffElement, err error) {
	liveObjs, err := cmdutil.LiveObjects(resources.Items)
	if err != nil {
		return false, nil, fmt.Errorf("Failed to get live objects: %w", err)
	}

	items := make([]objKeyLiveTarget, 0)
	var unstructureds []*unstructured.Unstructured
	for _, mfst := range diffOptions.res.Manifests {
		obj, err := argoappv1.UnmarshalToUnstructured(mfst)
		if err != nil {
			return false, nil, fmt.Errorf("Failed to unmarshal manifest: %w", err)
		}
		unstructureds = append(unstructureds, obj)
	}
	groupedObjs, err := groupObjsByKey(unstructureds, liveObjs, app.Spec.Destination.Namespace)
	if err != nil {
		return false, nil, fmt.Errorf("Failed to group objects by key: %w", err)
	}
	items, err = groupObjsForDiff(resources, groupedObjs, items, argoSettings, app.InstanceName(argoSettings.ControllerNamespace), app.Spec.Destination.Namespace)
	if err != nil {
		return false, nil, fmt.Errorf("Failed to group objects for diff: %w", err)
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
			return false, nil, fmt.Errorf("Failed to build diff config: %w", err)
		}
		diffRes, err := argodiff.StateDiff(item.live, item.target, diffConfig)
		if err != nil {
			return false, nil, fmt.Errorf("Failed to diff objects: %w", err)
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
					return false, nil, fmt.Errorf("Failed to unmarshal predicted live object: %w", err)
				}
			} else {
				live = item.live
				target = item.target
			}
			if !foundDiffs {
				foundDiffs = true
			}

			if keepDiffData {
				diffElement.Diff, err = diffLiveVsTargetObject(live, target)
			} else {
				diffElement.Diff = "✂️ ✂️  Redacted ✂️ ✂️ \nUnset component-level configuration key `disableArgoCDDiff` to see diff content."
			}
			if err != nil {
				return false, nil, fmt.Errorf("Failed to diff live objects: %w", err)
			}
		}
		diffElements = append(diffElements, diffElement)
	}
	return foundDiffs, diffElements, nil
}

// diffLiveVsTargetObject returns the diff of live and target in a format that
// is compatible with Github markdown diff highlighting.
func diffLiveVsTargetObject(live, target *unstructured.Unstructured) (string, error) {
	a, err := yaml2.Marshal(live)
	if err != nil {
		return "", err
	}
	b, err := yaml2.Marshal(target)
	if err != nil {
		return "", err
	}
	patch := diff.Diff(ctxLines, "live", a, "target", b)
	return string(patch), nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func createArgoCdClients() (ac argoCdClients, err error) {
	plaintext, _ := strconv.ParseBool(getEnv("ARGOCD_PLAINTEXT", "false"))
	insecure, _ := strconv.ParseBool(getEnv("ARGOCD_INSECURE", "false"))

	opts := &apiclient.ClientOptions{
		ServerAddr: getEnv("ARGOCD_SERVER_ADDR", "localhost:8080"),
		AuthToken:  getEnv("ARGOCD_TOKEN", ""),
		PlainText:  plaintext,
		Insecure:   insecure,
	}

	client, err := apiclient.NewClient(opts)
	if err != nil {
		return ac, fmt.Errorf("Error creating ArgoCD API client: %w", err)
	}

	_, ac.app, err = client.NewApplicationClient()
	if err != nil {
		return ac, fmt.Errorf("Error creating ArgoCD app client: %w", err)
	}

	_, ac.project, err = client.NewProjectClient()
	if err != nil {
		return ac, fmt.Errorf("Error creating ArgoCD project client: %w", err)
	}

	_, ac.setting, err = client.NewSettingsClient()
	if err != nil {
		return ac, fmt.Errorf("Error creating ArgoCD settings client: %w", err)
	}

	_, ac.appSet, err = client.NewApplicationSetClient()
	if err != nil {
		return ac, fmt.Errorf("Error creating ArgoCD appSet client: %w", err)
	}

	return
}

// This function will search for an ApplicationSet by the componentPath and repo name by comparing the componentPath with the ApplicationSet's spec.generators.[]git.directories
func findRelevantAppSetByPath(ctx context.Context, componentPath string, repo string, appSetClient applicationsetpkg.ApplicationSetServiceClient) (appSet *argoappv1.ApplicationSet, err error) {
	appSetQuery := applicationsetpkg.ApplicationSetListQuery{}

	foundAppSets, err := appSetClient.List(ctx, &appSetQuery)
	if err != nil {
		return nil, fmt.Errorf("Error listing ArgoCD ApplicationSets: %w", err)
	}
	for _, appSet := range foundAppSets.Items {
		for _, generator := range appSet.Spec.Generators {
			log.Debugf("Checking ApplicationSet %s for component path %s(repo %s)", appSet.Name, componentPath, repo)
			if generator.Git.RepoURL == repo {
				for _, dir := range generator.Git.Directories {
					match, _ := path.Match(dir.Path, componentPath)
					if match {
						log.Debugf("Found ArgoCD ApplicationSet %s for component path %s(repo %s)", appSet.Name, componentPath, repo)
						return &appSet, nil
					} else {
						log.Debugf("No match for %s in %s", componentPath, dir.Path)
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("No ArgoCD ApplicationSet found for component path %s(repo %s)", componentPath, repo)
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
		return nil, fmt.Errorf("Error listing ArgoCD applications: %w", err)
	}
	if len(foundApps.Items) == 0 {
		log.Infof("No ArgoCD application found for component path sha1 %s(repo %s), used this label selector: %s", componentPathSha1, repo, labelSelector)
		return nil, nil
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
	log.Infof("No ArgoCD application found with manifest-generate-paths annotation that matches %s(looked at repo %s, checked %v apps)", componentPath, repo, len(allRepoApps.Items))
	return nil, nil
}

func findArgocdApp(ctx context.Context, componentPath string, repo string, appClient application.ApplicationServiceClient, useSHALabelForArgoDicovery bool) (app *argoappv1.Application, err error) {
	f := findArgocdAppByManifestPathAnnotation
	if useSHALabelForArgoDicovery {
		f = findArgocdAppBySHA1Label
	}
	return f(ctx, componentPath, repo, appClient)
}

func SetArgoCDAppRevision(ctx context.Context, componentPath string, revision string, repo string, useSHALabelForArgoDicovery bool) error {
	var foundApp *argoappv1.Application
	var err error
	ac, err := createArgoCdClients()
	if err != nil {
		return fmt.Errorf("Error creating ArgoCD clients: %w", err)
	}
	foundApp, err = findArgocdApp(ctx, componentPath, repo, ac.app, useSHALabelForArgoDicovery)
	if err != nil {
		return fmt.Errorf("error finding ArgoCD application for component path %s: %w", componentPath, err)
	}
	if foundApp.Spec.Source.TargetRevision == revision {
		log.Infof("App %s already has revision %s", foundApp.Name, revision)
		return nil
	}

	patchObject := struct {
		Spec struct {
			Source struct {
				TargetRevision string `json:"targetRevision"`
			} `json:"source"`
		} `json:"spec"`
	}{}
	patchObject.Spec.Source.TargetRevision = revision
	patchJson, _ := json.Marshal(patchObject)
	patch := string(patchJson)
	log.Debugf("Patching app %s/%s with: %s", foundApp.Namespace, foundApp.Name, patch)

	patchType := "merge"
	_, err = ac.app.Patch(ctx, &application.ApplicationPatchRequest{
		Name:         &foundApp.Name,
		AppNamespace: &foundApp.Namespace,
		PatchType:    &patchType,
		Patch:        &patch,
	})
	if err != nil {
		return fmt.Errorf("revision patching failed: %w", err)
	} else {
		log.Infof("ArgoCD App %s revision set to %s", foundApp.Name, revision)
	}

	return err
}

// copied form https://github.com/argoproj/argo-cd/blob/v2.11.4/applicationset/controllers/applicationset_controller.go#L493C1-L503C2
func getTempApplication(applicationSetTemplate argoappv1.ApplicationSetTemplate) *argoappv1.Application {
	var tmplApplication argoappv1.Application
	tmplApplication.Annotations = applicationSetTemplate.Annotations
	tmplApplication.Labels = applicationSetTemplate.Labels
	tmplApplication.Namespace = applicationSetTemplate.Namespace
	tmplApplication.Name = applicationSetTemplate.Name
	tmplApplication.Spec = applicationSetTemplate.Spec
	tmplApplication.Finalizers = applicationSetTemplate.Finalizers

	return &tmplApplication
}

// This function generate the params map for the ApplicationSet template, mimicking the behavior of the ApplicationSet controller Git Generator
func generateAppSetGitGeneratorParams(p string) map[string]interface{} {
	params := make(map[string]interface{})
	paramPath := map[string]interface{}{}

	paramPath["path"] = p
	paramPath["basename"] = path.Base(paramPath["path"].(string))
	paramPath["filename"] = path.Base(p)
	paramPath["basenameNormalized"] = utils.SanitizeName(path.Base(paramPath["path"].(string)))
	paramPath["filenameNormalized"] = utils.SanitizeName(path.Base(paramPath["filename"].(string)))
	paramPath["segments"] = strings.Split(paramPath["path"].(string), "/")
	params["path"] = paramPath
	return params
}

func createTempAppObjectFroNewApp(ctx context.Context, componentPath string, repo string, prBranch string, ac argoCdClients) (app *argoappv1.Application, err error) {
	log.Debug("Didn't find ArgoCD App, trying to find a relevant  ApplicationSet")
	appSetOfcomponent, err := findRelevantAppSetByPath(ctx, componentPath, repo, ac.appSet)
	if appSetOfcomponent != nil {
		useGoTemplate := true
		var goTemplateOptions []string
		params := generateAppSetGitGeneratorParams(componentPath)
		r := &utils.Render{}
		newAppObject, err := r.RenderTemplateParams(getTempApplication(appSetOfcomponent.Spec.Template), nil, params, useGoTemplate, goTemplateOptions)
		if err != nil {
			log.Errorf("params: %v", params)
			log.Errorf("Error rendering ApplicationSet template: %v", err)
		}

		// Mutating some of the app object fields to fit this specific use case
		tempAppName := fmt.Sprintf("temp-%s", newAppObject.Name)
		newAppObject.Name = tempAppName
		// We need to remove the automated sync policy, we just want to create a temporary app object, run a diff and remove it.
		newAppObject.Spec.SyncPolicy.Automated = nil
		newAppObject.Spec.Source.TargetRevision = prBranch

		validateTempApp := false
		appCreateRequest := application.ApplicationCreateRequest{
			Application: newAppObject,
			Validate:    &validateTempApp, // It makes more sense to handle template failures in the diff generation section
		}
		// Create the temporary app object
		app, err = ac.app.Create(ctx, &appCreateRequest)

		return app, err
	} else {
		return nil, err
	}
}

func generateDiffOfAComponent(ctx context.Context, commentDiff bool, componentPath string, prBranch string, repo string, ac argoCdClients, argoSettings *settings.Settings, useSHALabelForArgoDicovery bool, createTempAppObjectFromNewApps bool) (componentDiffResult DiffResult) {
	componentDiffResult.ComponentPath = componentPath

	// Find ArgoCD application by the path SHA1 label selector and repo name
	// At the moment we assume one to one mapping between Telefonistka components and ArgoCD application

	var app *argoappv1.Application
	var err error
	if useSHALabelForArgoDicovery {
		app, err = findArgocdAppBySHA1Label(ctx, componentPath, repo, ac.app)
		if err != nil {
			componentDiffResult.DiffError = err
			return componentDiffResult
		}
	} else {
		app, err = findArgocdAppByManifestPathAnnotation(ctx, componentPath, repo, ac.app)
		if err != nil {
			componentDiffResult.DiffError = err
			return componentDiffResult
		}
	}
	if app == nil {
		if createTempAppObjectFromNewApps {
			app, err = createTempAppObjectFroNewApp(ctx, componentPath, repo, prBranch, ac)

			if err != nil {
				log.Errorf("Error creating temporary app object: %v", err)
				componentDiffResult.DiffError = err
				return componentDiffResult
			} else {
				log.Debugf("Created temporary app object: %s", app.Name)
				componentDiffResult.AppWasTemporarilyCreated = true
			}
		} else {
			componentDiffResult.DiffError = fmt.Errorf("No ArgoCD application found for component path %s(repo %s)", componentPath, repo)
			return
		}
	} else {
		// Get the application and its resources, resources are the live state of the application objects.
		// The 2nd "app fetch" is needed for the "refreshTypeHard", we don't want to do that to non-relevant apps"
		refreshType := string(argoappv1.RefreshTypeHard)
		appNameQuery := application.ApplicationQuery{
			Name:    &app.Name, // we expect only one app with this label and repo selectors
			Refresh: &refreshType,
		}
		app, err := ac.app.Get(ctx, &appNameQuery)
		if err != nil {
			componentDiffResult.DiffError = err
			log.Errorf("Error getting app(HardRefresh) %s: %v", app.Name, err)
			return componentDiffResult
		}
		log.Debugf("Got ArgoCD app %s", app.Name)
	}
	componentDiffResult.ArgoCdAppName = app.Name
	componentDiffResult.ArgoCdAppURL = fmt.Sprintf("%s/applications/%s", argoSettings.URL, app.Name)

	if app.Spec.Source.TargetRevision == prBranch && app.Spec.SyncPolicy.Automated != nil {
		componentDiffResult.DiffError = fmt.Errorf("App %s already has revision %s as Source Target Revision and autosync is on, skipping diff calculation", app.Name, prBranch)
		return componentDiffResult
	}

	resources, err := ac.app.ManagedResources(ctx, &application.ResourcesQuery{ApplicationName: &app.Name, AppNamespace: &app.Namespace})
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
	manifests, err := ac.app.GetManifests(ctx, &manifestQuery)
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting manifests for app %s, revision %s: %v", app.Name, prBranch, err)
		return componentDiffResult
	}
	log.Debugf("Got manifests for app %s, revision %s", app.Name, prBranch)
	diffOption.res = manifests
	diffOption.revision = prBranch

	// Now we diff the live state(resources) and target state of the application objects(diffOption.res)
	detailedProject, err := ac.project.GetDetailedProject(ctx, &projectpkg.ProjectQuery{Name: app.Spec.Project})
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting project %s: %v", app.Spec.Project, err)
		return componentDiffResult
	}

	log.Debugf("Generating diff for component %s", componentPath)
	componentDiffResult.HasDiff, componentDiffResult.DiffElements, componentDiffResult.DiffError = generateArgocdAppDiff(ctx, commentDiff, app, detailedProject.Project, resources, argoSettings, diffOption)

	// only delete the temprorary app object if it was created and there was no error on diff
	// otherwise let's keep it for investigation
	if componentDiffResult.AppWasTemporarilyCreated && componentDiffResult.DiffError == nil {
		// Delete the temporary app object
		_, err = ac.app.Delete(ctx, &application.ApplicationDeleteRequest{Name: &app.Name, AppNamespace: &app.Namespace})
		if err != nil {
			log.Errorf("Error deleting temporary app object: %v", err)
			componentDiffResult.DiffError = err
		} else {
			log.Debugf("Deleted temporary app object: %s", app.Name)
		}
	}

	return componentDiffResult
}

// GenerateDiffOfChangedComponents generates diff of changed components
func GenerateDiffOfChangedComponents(ctx context.Context, componentsToDiff map[string]bool, prBranch string, repo string, useSHALabelForArgoDicovery bool, createTempAppObjectFromNewApps bool) (hasComponentDiff bool, hasComponentDiffErrors bool, diffResults []DiffResult, err error) {
	hasComponentDiff = false
	hasComponentDiffErrors = false
	// env var should be centralized
	ac, err := createArgoCdClients()
	if err != nil {
		log.Errorf("Error creating ArgoCD clients: %v", err)
		return false, true, nil, err
	}

	argoSettings, err := ac.setting.Get(ctx, &settings.SettingsQuery{})
	if err != nil {
		log.Errorf("Error getting ArgoCD settings: %v", err)
		return false, true, nil, err
	}

	for componentPath, shouldIDiff := range componentsToDiff {
		currentDiffResult := generateDiffOfAComponent(ctx, shouldIDiff, componentPath, prBranch, repo, ac, argoSettings, useSHALabelForArgoDicovery, createTempAppObjectFromNewApps)
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
