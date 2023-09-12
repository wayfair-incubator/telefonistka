package argocd

import (
	argoclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	cfg "github.com/wayfair-incubator/telefonistka/internal/pkg/configuration"
)

type argoCDApp struct {
	appName           string
	appNamespace      string
	ArgocdInstanceURL string
}

func generateListOfArgoCDApps() []argoCDApp {
	// TODO actually get app name and ArgoCD URL!!!
	return []argoCDApp{
		argoCDApp{
			appName:           "sandbox-emoji-demo-web",
			appNamespace:      "argocd",
			ArgocdInstanceURL: "https://kube-argocd-c1.service.intradsm1.sdedevconsul.csnzoo.com",
		},
	}

}

// Posts ArgoCD diff on a PR comment
func ShowArgocdDiff(config *cfg.Config) error {

	// changedComponents := githubapi.GenerateRelevantComponentsLisit(prFiles, config)
	opts := argoclient.ClientOptions{}
	argoclient.NewClient(opts)

	return nil // TODO
}
