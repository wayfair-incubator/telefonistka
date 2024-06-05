package argocd

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: Blocklisted import crypto/sha1: weak cryptographic primitive (gosec), this is not a cryptographic use case
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	cmdutil "github.com/argoproj/argo-cd/v2/cmd/util"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/settings"
	argoappv1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	argodiff "github.com/argoproj/argo-cd/v2/util/argo/diff"
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
		return false, nil, err
	}

	items := make([]objKeyLiveTarget, 0)
	var unstructureds []*unstructured.Unstructured
	for _, mfst := range diffOptions.res.Manifests {
		obj, err := argoappv1.UnmarshalToUnstructured(mfst)
		if err != nil {
			return false, nil, err
		}
		unstructureds = append(unstructureds, obj)
	}
	groupedObjs := groupObjsByKey(unstructureds, liveObjs, app.Spec.Destination.Namespace)
	items = groupObjsForDiff(resources, groupedObjs, items, argoSettings, app.InstanceName(argoSettings.ControllerNamespace), app.Spec.Destination.Namespace)

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
		diffConfig, err := argodiff.NewDiffConfigBuilder().
			WithDiffSettings(app.Spec.IgnoreDifferences, overrides, ignoreAggregatedRoles).
			WithTracking(argoSettings.AppLabelKey, argoSettings.TrackingMethod).
			WithNoCache().
			Build()
		if err != nil {
			return false, nil, err
		}
		diffRes, err := argodiff.StateDiff(item.live, item.target, diffConfig)
		if err != nil {
			return false, nil, err
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
					return false, nil, err
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
				return false, nil, err
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
		return nil, err
	}
	return clientset, nil
}

func generateDiffOfAComponent(ctx context.Context, componentPath string, prBranch string, repo string, appIf application.ApplicationServiceClient, projIf projectpkg.ProjectServiceClient, argoSettings *settings.Settings) (componentDiffResult DiffResult) {
	componentDiffResult.ComponentPath = componentPath

	// Calculate sha1 of component path to use in a label selector
	cPathBa := []byte(componentPath)
	hasher := sha1.New() //nolint:gosec // G505: Blocklisted import crypto/sha1: weak cryptographic primitive (gosec), this is not a cryptographic use case
	hasher.Write(cPathBa)
	componentPathSha1 := hex.EncodeToString(hasher.Sum(nil))

	// Find ArgoCD application by the path SHA1 label selector and repo name
	// That label is assumed to be pupulated by the ApplicationSet controller(or apps of apps  or similar).
	labelSelector := fmt.Sprintf("telefonistka.io/component-path-sha1=%s", componentPathSha1)
	log.Debugf("Using label selector: %s", labelSelector)
	appLabelQuery := application.ApplicationQuery{
		Selector: &labelSelector,
		Repo:     &repo,
	}
	foundApps, err := appIf.List(ctx, &appLabelQuery)
	if err != nil {
		log.Errorf("Error listing ArgoCD applications: %v", err)
		componentDiffResult.DiffError = err
		return componentDiffResult
	}
	if len(foundApps.Items) == 0 {
		componentDiffResult.DiffError = fmt.Errorf("No ArgoCD application found for component path %s(repo %s), used this label selector: %s", componentPath, repo, labelSelector)
		return componentDiffResult
	}

	log.Debugf("Found ArgoCD application: %s", foundApps.Items[0].Name)
	// Get the application and its resources, resources are the live state of the application objects.
	refreshType := string(argoappv1.RefreshTypeHard)
	appNameQuery := application.ApplicationQuery{
		Name:    &foundApps.Items[0].Name, // we expect only one app with this label and repo selectors
		Refresh: &refreshType,
	}
	app, err := appIf.Get(ctx, &appNameQuery)
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting app %s: %v", foundApps.Items[0].Name, err)
		return componentDiffResult
	}
	log.Debugf("Got ArgoCD app %s", app.Name)
	componentDiffResult.ArgoCdAppName = app.Name
	componentDiffResult.ArgoCdAppURL = fmt.Sprintf("%s/applications/%s", argoSettings.URL, app.Name)
	resources, err := appIf.ManagedResources(ctx, &application.ResourcesQuery{ApplicationName: &app.Name, AppNamespace: &app.Namespace})
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
	manifests, err := appIf.GetManifests(ctx, &manifestQuery)
	if err != nil {
		componentDiffResult.DiffError = err
		log.Errorf("Error getting manifests for app %s, revision %s: %v", app.Name, prBranch, err)
		return componentDiffResult
	}
	log.Debugf("Got manifests for app %s, revision %s", app.Name, prBranch)
	diffOption.res = manifests
	diffOption.revision = prBranch

	// Now we diff the live state(resources) and target state of the application objects(diffOption.res)
	detailedProject, err := projIf.GetDetailedProject(ctx, &projectpkg.ProjectQuery{Name: app.Spec.Project})
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
func GenerateDiffOfChangedComponents(ctx context.Context, componentPathList []string, prBranch string, repo string) (hasComponentDiff bool, hasComponentDiffErrors bool, diffResults []DiffResult, err error) {
	hasComponentDiff = false
	hasComponentDiffErrors = false
	// env var should be centralized
	client, err := createArgoCdClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD client: %v", err)
		return false, true, nil, err
	}

	conn, appIf, err := client.NewApplicationClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD app client: %v", err)
		return false, true, nil, err
	}
	defer argoio.Close(conn)

	conn, projIf, err := client.NewProjectClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD project client: %v", err)
		return false, true, nil, err
	}
	defer argoio.Close(conn)

	conn, settingsIf, err := client.NewSettingsClient()
	if err != nil {
		log.Errorf("Error creating ArgoCD settings client: %v", err)
		return false, true, nil, err
	}
	defer argoio.Close(conn)
	argoSettings, err := settingsIf.Get(ctx, &settings.SettingsQuery{})
	if err != nil {
		log.Errorf("Error getting ArgoCD settings: %v", err)
		return false, true, nil, err
	}

	log.Debugf("Checking ArgoCD diff for components: %v", componentPathList)
	for _, componentPath := range componentPathList {
		currentDiffResult := generateDiffOfAComponent(ctx, componentPath, prBranch, repo, appIf, projIf, argoSettings)
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
