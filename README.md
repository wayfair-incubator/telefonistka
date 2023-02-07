<!-- markdownlint-disable MD033 -->
<p align="center">  <img src="https://user-images.githubusercontent.com/1616153/217223509-9aa60a5c-a263-41a7-814d-a1bf2957acf6.png " width="150"> </p>

# <p align="center"> Telefonistka</p>

<!-- markdownlint-enable MD033 -->

Telefonistka is a Github webhook server/Bot that facilitate change promotion across environments/failure domains in IaC GitOps repos.

It assumes the [the repeatable part if your infrastucture is modeled in folders](#modeling-environmentsfailure-domains-in-an-iac-gitops-repo)

Based on configuration in the IaC repo, the bot will open Pull Requests that syncs components from "sourcePath"s to "targetPaths".

Providing reasonably flexible control over what is promoted to where and in what order.

## Modeling environments/failure domains in an IaC GitOps repo

RY is the new DRY!

Regardless of the tool you use to describe your infrastructure, or if your IaC repo includes code or just references to some versioned artifacts like helm charts/TF modules, you need a way to control how changes are made across environments("dev"/"prod"/...) and failure domains("us-east-1"/"us-west-1"/...).

If changes are applied immediately when they are committed to the repo, this means these  environments and failure domains need to be represented as different folders or branches to provide said control.

While using Git branches allows using git native tools for promoting changes(git merge) and inspecting drift(git diff) it quickly becomes cumbersome as the number of distinct environment/FDs grows. Additionally, syncing all your infrastructure from the main branch keeps the GitOps side of things more intuitive and make the promotion side more observable.

This leaves us with "the folders" approach, while gaining simplicity and observability it would requires us to manually copy files around, (r)sync directories or even worse - manually make the same change in multiple files.

This is where Telefonistka comes in.

## Notable Features

* IaC technology agnostic - Terraform, Helmfile, ArgoCD whatever, as long as environments and sites are modeled as folders and components are copied "as is".
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

* Control over grouping of targetPaths syncs in PRs ("Sync all dev clusters in one PR but open a dedicated PR for every production cluster" )
* Optional in-component allow/block override list("This component should not be deployed to production" or "Deploy this only in the us-east-4 region")
* Drift detection - warns user on "unsynced" environment on open PRs ("Staging the Production are not synced, these are the differences")

## Server Configuration

Environment variables for the webhook process:

`APPROVER_GITHUB_OAUTH_TOKEN` GitHub oAuth token for automatically approving promotion PRs

`GITHUB_OAUTH_TOKEN` GitHub main oAuth token for all other GH operations

`GITHUB_URL` URL for github API (needed for Github Enterprise)

`GITHUB_WEBHOOK_SECRET` secret used to sign webhook payload to be validated by the WH server, must match the sting in repo settings/hooks page

Behavior of the bot is configured by YAML files **in the target repo**:

## Repo Configuration

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
        - "clusters/dev/us-east4/c2"
        - "clusters/lab/europe-west4/c1"
        - "clusters/staging/us-central1/c1"
        - "clusters/staging/us-central1/c2"
        - "clusters/staging/europe-west4/c1"
  - sourcePath: "clusters/staging/[^/]*/[^/]*" # This will start a promotion to prod from any "staging" path
    conditions:
      prHasLabels:
        - "quick_promotion" # This flow will run only if PR has "quick_promotion" label, see targetPaths below
    targetPaths:
      -
        - "clusters/prod/us-west1/c2" # First PR for only a single cluster
      -
        - "clusters/prod/europe-west3/c2" # 2nd PR will sync all 4 remaining clusters
        - "clusters/prod/europe-west4/c2"
        - "clusters/prod/us-central1/c2"
        - "clusters/prod/us-east4/c2"
  - sourcePath: "clusters/staging/[^/]*/[^/]*" # This flow will run on PR without "quick_promotion" label
    targetPaths:
      -
        - "clusters/prod/us-west1/c2" # Each cluster will have its own promotion PR
      -
        - "clusters/prod/europe-west3/c2"
      -
        - "clusters/prod/europe-west4/c2"
      -
        - "clusters/prod/us-central1/c2"
      -
        - "clusters/prod/us-east4/c2"
dryRunMode: true
autoApprovePromotionPrs: true
toggleCommitStatus:
  override-terrafrom-pipeline: "github-action-terraform"
```

## Component Configuration

This optional in-component configuration file allows overriding the general promotion configuration for a specific component.  
File location is `COMPONENT_PATH/telefonistka.yaml` (no leading dot in file name), so it could be:  
`workspace/reloader/telefonistka.yaml` or `env/prod/us-central1/c2/wf-kube-proxy-metrics-proxy/telefonistka.yaml`  
it includes only two optional configuration keys, `promotionTargetBlockList` and `promotionTargetAllowList`.  
Both are matched against the target component path using Golang regex engine.

If a target path matches an entry in `promotionTargetBlockList` it will not be promoted(regardless of `promotionTargetAllowList`).

If  `promotionTargetAllowList` exist(non empty), only target paths that matches it will be promoted to(but the previous statement about `promotionTargetBlockList` still applies).

```yaml
promotionTargetBlockList:
  - env/staging/europe-west4/c1.*
  - env/prod/us-central1/c3/
promotionTargetAllowList:
  - env/prod/.*
  - env/(dev|lab)/.*
```

## Metrics

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

## Development

* use Ngrok ( `ngrok http 8080` ) to expose the local instance
* See the URLs in ngrok command output.
* Add a webhook to repo setting (don't forget the `/webhook` path in the URL).
* Content type needs to be `application/json`, **currently** only PR events are needed

## Installation

TODO

## Roadmap

See the [open issues](https://github.com/wayfair-incubator/telefonistka/issues) for a list of proposed features (and known issues).

## FAQ

* Why is this deployed as a webhook server and not a CI/CD plugin like Github Actions? - Modern CI/CD system like GH actions and CircleCI usually allow unapproved code/configuration to execute on branches/PRs, this makes securing them or the secret they need somewhat hard.  
Telefonistka needs credentials that allows opening PRs and approving them which in an IaC GitOps repo represent significant power. Running it as a distinct workload in a VM/container provides better security.  
That being said, we acknowledge that maintaining an additional piece of infrastructure might not be for everyone, especially if you need it for only one repo so we do plan to release a Github action based version in the future.

## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**. For detailed contributing guidelines, please see [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.
