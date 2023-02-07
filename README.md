<p align="center">  <img src="https://user-images.githubusercontent.com/1616153/217223509-9aa60a5c-a263-41a7-814d-a1bf2957acf6.png " width="150"> </p>

# <p align="center"> Telefonistka</p>




Telefonistka is a Github Webhook Bot that facilitate promotions in a IaC GitOps repo that models environments and sites as folders.

Based on configuration in the IaC repo, the bot will open Pull Requests that syncs components from "sourcePath"s to "targetPaths".

Providing reasonably flexible control over what is promoted to where and in what order.

### Notable Features

* IaC technology agnostic -  Terraform, Helmfile, ArgoCD whatever, as long as environments and sites are modeled as folders and components are copied "as is".

* Multi stage promotion schemes like  

  ```text
  lab -> staging -> production
  ```

  or  

  ```text
  dev -> production-us-east-1 -> production-us-east-3 -> production-eu-east-1
  ```  
  
  Fan out, like:  

  ```text
  lab -> staging1 -->
         staging2 -->  production
         staging3 -->
  ```

* Control over grouping of targetPaths syncs in PRs ("sync all dev clusters in one PR but open a dedicated PR for every production cluster" )
* Optional in-component allow/block override list("this component should not be deployed to production" or "deploy this only in the us-east-4 region")
* Drift detection - warns user on "unsynced" environment on open PRs ("Staging the Production are not synced, these are the differences")

### Server Configuration

Environment variables for the webhook process:

`APPROVER_GITHUB_OAUTH_TOKEN` GitHub oAuth token for automatically approving promotion PRs

`GITHUB_OAUTH_TOKEN` GitHub main oAuth token for all other GH operations

`GITHUB_URL` URL for github API (needed for Github Enterprise)

`GITHUB_WEBHOOK_SECRET` secret used to sign webhook payload to be validated by the WH server, must match the sting in repo settings/hooks page

Behavior of the bot is configured by YAML files **in the target repo**:

### Repo Configuration

Pulled from `telefonistka.yaml` file in the repo root directory(default branch)

Configuration keys:  

|key|desc|
|---|---|
|`promotionPaths`| Array of maps, each map describes a promotion flow|  
|`promotionPaths[0].sourcePath`| directory that holds components(subdirectories) to be synced, can include a regex.|
|`promotionPaths[0].conditions` | conditions for triggering a specific promotion flows. Flows are evatluated in order, first one to match is triggered.|
|`promotionPaths[0].conditions.prHasLabels` | Array of PR labels, if the triggering PR has any of these lables the condition is considered fulfilled. Currently it's the only supported condition type|
|`promotionPaths[0].targetPaths`|  Array of arrays(!!!) of target paths tied to the source path mentioned above, each top level element represent a PR that will be opened, so multiple target can be synced in a single PR|  
|`dryRunMode`| if true, the bot will just comment the planned promotion on the merged PR|
|`autoApprovePromotionPrs`| if true the bot will auto-approve all promotion PRs, with the assumption the original PR was peer reviewed and is promoted verbatim. Required additional GH token via APPROVER_GITHUB_OAUTH_TOKEN env variable|
|`toggleCommitStatus`| Map of strings, allow (non-repo-admin) users to change the [Github commit status](https://docs.github.com/en/rest/commits/statuses) state(from failure to success and back). This can be used to continue promotion of a change that doesn't pass repo checks. the keys are strings commented in the PRs, values are [Github commit status context](https://docs.github.com/en/rest/commits/statuses?apiVersion=2022-11-28#create-a-commit-status) to be overridden|

Example:

```yaml
promotionPaths:
  - sourcePath: "workspace/"
    targetPaths:
      - 
        - "clusters/dev/iad1/c2"
        - "clusters/sdedev/grq1/c1"
        - "clusters/sdeprod/dsm1/c1"
        - "clusters/sdeprod/dsm1/c2"
        - "clusters/sdeprod/grq1/c1"
  - sourcePath: "clusters/sdeprod/[^/]*/[^/]*" # This will start a promotion to prod from any "sdeprod" path
    conditions:
      prHasLabels:
        - "quick_promotion" # This flow will run only if PR has "quick_promotion" label, see targetPaths below
    targetPaths:
      -
        - "clusters/prod/pdx1/c2" # First PR for only a single cluster
      -
        - "clusters/prod/fra1/c2" # 2nd PR will sync all 4 remaining clusters
        - "clusters/prod/grq1/c2"
        - "clusters/prod/dsm1/c2"
        - "clusters/prod/iad1/c2"
  - sourcePath: "clusters/sdeprod/[^/]*/[^/]*" # This flow will run on PR without "quick_promotion" label
    targetPaths:
      -
        - "clusters/prod/pdx1/c2" # Each cluster will have its own promotion PR
      -
        - "clusters/prod/fra1/c2"
      -
        - "clusters/prod/grq1/c2"
      -
        - "clusters/prod/dsm1/c2"
      -
        - "clusters/prod/iad1/c2"
dryRunMode: true
autoApprovePromotionPrs: true
toggleCommitStatus:
  override-terrafrom-pipeline: "github-action-terraform"
```

### Component Configuration

This optional in-component configuation file allows overriding the general promotion configuation for a specific component.  
File location is `COMPONENT_PATH/telefonistka.yaml` (no leading dot in file name), so it could be:  
`workspace/reloader/telefonistka.yaml` or `env/prod/dsm1/c2/wf-kube-proxy-metrics-proxy/telefonistka.yaml`  
it includes only two optional configuation keys, `promotionTargetBlockList` and `promotionTargetAllowList`.  
Both are matched against the target component path using Golang regex engine.

If a target path matches an entry in `promotionTargetBlockList` it will not be promoted(regardless of `promotionTargetAllowList`).

If  `promotionTargetAllowList` exist(non empty), only target paths that matches it will be promoted to(but the previous statement about `promotionTargetBlockList` still applies).

```yaml
promotionTargetBlockList:
  - env/sdeprod/grq1/c1.*
  - env/prod/dsm1/c3/
promotionTargetAllowList:
  - env/prod/.*
  - env/sde.*
```

### Metrics

```text
# HELP telefonistka_github_github_operations_total The total number of Github operations
# TYPE telefonistka_github_github_operations_total counter
telefonistka_github_github_operations_total{api_group="repos",api_path="",method="GET",repo_slug="shared/k8s-helmfile",status="200"} 8
telefonistka_github_github_operations_total{api_group="repos",api_path="contents",method="GET",repo_slug="shared/k8s-helmfile",status="200"} 76
telefonistka_github_github_operations_total{api_group="repos",api_path="contents",method="GET",repo_slug="shared/k8s-helmfile",status="404"} 13
telefonistka_github_github_operations_total{api_group="repos",api_path="issues",method="POST",repo_slug="shared/k8s-helmfile",status="201"} 3
telefonistka_github_github_operations_total{api_group="repos",api_path="pulls",method="GET",repo_slug="shared/k8s-helmfile",status="200"} 8
# HELP telefonistka_github_github_rest_api_client_rate_limit The number of requests per hour the client is currently limited to
# TYPE telefonistka_github_github_rest_api_client_rate_limit gauge
telefonistka_github_github_rest_api_client_rate_limit 100000
# HELP telefonistka_github_github_rest_api_client_rate_remaining The number of remaining requests the client can make this hour
# TYPE telefonistka_github_github_rest_api_client_rate_remaining gauge
telefonistka_github_github_rest_api_client_rate_remaining 99668
# HELP telefonistka_webhook_server_webhook_hits_total The total number of validated webhook hits
# TYPE telefonistka_webhook_server_webhook_hits_total counter
telefonistka_webhook_server_webhook_hits_total{parsing="successful"} 8
```

### Development

* use Ngrok ( `ngrok http 8080` ) to expose the local instance
* See the URLs in ngrok command output.
* Add a webhook to repo setting e.g. `https://github.csnzoo.com/ob136j/k8s-gitops-poc/settings/hooks`
(don't forget the `/webhook` path in the URL).
* Content type needs to be `application/json`, **currently** only PR events are needed

### Installation

TODO

## Roadmap

See the [open issues](https://github.com/wayfair-incubator/telefonistka/issues) for a list of proposed features (and known issues).

## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**. For detailed contributing guidelines, please see [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.
