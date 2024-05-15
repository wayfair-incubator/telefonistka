## Installation

Telefonistka can run as a webhook server or via GitHub Action.
It is much easier to setup the GitHub Action configuration but the VM and container provisioning add a very noticeable delay to the event handling.

Example GitHub Actions configuration:

```yaml
name: Telefonistka
# Controls when the workflow will run "closed" is used for prmoting changes, "opened"/"synchronize" are needed to warn on drift.
on:
  # Triggers the workflow on push or pull request events but only for the "main" branch
  pull_request:
    types: 
      - closed
      - opened
      - synchronize
    branches: [ "main" ]

jobs:
  telefonistka:
    runs-on: ubuntu-latest
    steps:
      - uses:  Oded-B/telefonistka-action@main
        with:
          repo-token: ${{ secrets.GITHUB_TOKEN }}
```

Additionally, GitHub Actions would need permission to write to  `content`, `pull-requests`, `statuses`.
This is done via the repository setting "Action" > "General" page:

<!-- markdownlint-disable MD033 -->
<img width="792" alt="image" src="https://user-images.githubusercontent.com/1616153/227661321-7118fde6-6ada-4f98-a406-f5d53023cef3.png">
<!-- markdownlint-enable MD033 -->

For optimal user expirience Telefonistka could be run as continuously running webhook server.
The current version of the docs doesn't cover the details of running the Telefonistka server beyond listing its [configuration options](#server-configuration) and noting that its `/webhook` endpoint needs to be accessible from Github(Cloud or private instance)

The Github side of the configuration can be done via a creation of an GitHub Application(recommended) or by configuring a webhook + github service account permission for each relevant repo.

### GitHub Application

* Create the application.
  * Go to GitHub [apps page](https://github.com/settings/apps) under "Developer settings" and create a new app.
  * Set the Webhooks URL to point to your running Telefonistka instance(remember the `/webhook` URL path), use HTTPS and set `Webhook secret` (pass  to instance via `GITHUB_WEBHOOK_SECRET` env var)
  * Provide the new app with read&write `Repository permissions` for `Commit statuses`, `Contents`, `Issues` and `Pull requests`.
  * Subscribe to `Issues` and `Pull request` events
  * Generate a `Private key` and provide it to your instance with the  `GITHUB_APP_PRIVATE_KEY_PATH` env variable.
  * Grab the `App ID`, provide it to your instance with the `GITHUB_APP_ID` env variable.
* For each relevant repo:
  * Add repo to application configuration.
  * Add `telefonistka.yaml` to repo root.

### Webhook + service account

* Create a github service account(basically a regular account) and generate an API Token, provide token via `GITHUB_OAUTH_TOKEN` Env var
* For each relevant repo:
  * Add the Telefonistka API endpoints to the webhooks under the repo settings page.
  * Ensure the service account has the relevant permission on the repo.
  * Add `telefonistka.yaml` to repo root.

## Images

Telefonistka comes in 2 flavors:

* A normal container image, that is built from `scratch` and contains only a telefonistka binary and CA certificates.
  This image is preferred and meant to be used in a production environment.
* A container image that is built on `alpine` and contains the full alpine base OS. Denoted by the `alpine-` prefix.
  This image is meant for development and debugging purposes.
  And in rare cases when it is used in CI and requires shell variable expansion.

## Server Configuration

Environment variables for the webhook process:

`APPROVER_GITHUB_OAUTH_TOKEN` GitHub OAuth token for automatically approving promotion PRs

`GITHUB_OAUTH_TOKEN` GitHub main OAuth token for all other GH operations

`GITHUB_HOST` Host name for github API, needed for Github Enterprise Server, should not include http scheme and path, e.g. :`my-gh-host.com`

`GITHUB_WEBHOOK_SECRET` secret used to sign webhook payload to be validated by the WH server, must match the sting in repo settings/hooks page

`GITHUB_APP_PRIVATE_KEY_PATH`  Private key for Github applications style of deployments, in PEM format

`GITHUB_APP_ID` Application ID for Github applications style of deployments, available in the Github Application setting page.

`TEMPLATES_PATH` Telefonistka uses Go templates to format GitHub PR comments, the variable override the default templates path("templates/"), useful for environments where the container workdir is overridden(like GitHub Actions) or when custom templates are desired.

`ARGOCD_SERVER_ADDR` Hostname and port of the ArgoCD API endpoint, like `argocd-server.argocd.svc.cluster.local:443`, default is `localhost:8080"`

`ARGOCD_TOKEN` ArgoCD authentication token.

`ARGOCD_PLAINTEXT` Allows disabeling TLS on ArgoCD API calls. (default: `false`)

`ARGOCD_INSECURE` Allow disabeling server certificate validation. (default: `false`)

Behavior of the bot is configured by YAML files **in the target repo**:

## Repo Configuration

Pulled from `telefonistka.yaml` file in the repo root directory(default branch)

Configuration keys:

<!-- markdownlint-disable MD033 -->
|key|desc|
|---|---|
|`promotionPaths`| Array of maps, each map describes a promotion flow|
|`promotionPaths[0].sourcePath`| directory that holds components(subdirectories) to be synced, can include a regex.|
|`promotionPaths[0].componentPathExtraDepth`| The number of extra nesting levels to add to the "components" being promoted, this allows nesting components in subdirectories while keeping them distinct.<br>A `2` value will mean the component name includes the 3 subdirectories under the `sourcePath`|
|`promotionPaths[0].conditions` | conditions for triggering a specific promotion flows. Flows are evaluated in order, first one to match is triggered.|
|`promotionPaths[0].conditions.prHasLabels` | Array of PR labels, if the triggering PR has any of these labels the condition is considered fulfilled.|
|`promotionPaths[0].conditions.autoMerge`| Boolean value. If set to true, PR will be automatically merged after it is created.|
|`promotionPaths[0].promotionPrs`|  Array of structures, each element represent a PR that will be opened when files are changed under `sourcePath`. Multiple elements means multiple PR will be opened|
|`promotionPaths[0].promotionPrs[0].targetPaths`| Array of strings, each element represent a directory to by synced from the changed component under  `sourcePath`. Multiple elements means multiple directories will be synced in a PR|
|`dryRunMode`| if true, the bot will just comment the planned promotion on the merged PR|
|`autoApprovePromotionPrs`| if true the bot will auto-approve all promotion PRs, with the assumption the original PR was peer reviewed and is promoted verbatim. Required additional GH token via APPROVER_GITHUB_OAUTH_TOKEN env variable|
|`toggleCommitStatus`| Map of strings, allow (non-repo-admin) users to change the [Github commit status](https://docs.github.com/en/rest/commits/statuses) state(from failure to success and back). This can be used to continue promotion of a change that doesn't pass repo checks. the keys are strings commented in the PRs, values are [Github commit status context](https://docs.github.com/en/rest/commits/statuses?apiVersion=2022-11-28#create-a-commit-status) to be overridden|
|`commentArgocdDiffonPR`| Uses ArgoCD API to calculate expected changes to k8s state and comment the resulting "diff" as comment in the PR. Requires ARGOCD_* environment variables, see below. |
|`autoMergeNoDiffPRs`| if true, Telefonistka will **merge** promotion PRs that are not expected to change the target clusters. Requires `commentArgocdDiffonPR` and possibly `autoApprovePromotionPrs`(depending on repo branch protection rules)|
<!-- markdownlint-enable MD033 -->

Example:

```yaml
promotionPaths:
  - sourcePath: "workspace/"
    conditions:
      autoMerge: true
    promotionPrs:
      - targetPaths:
        - "clusters/dev/us-east4/c2"
        - "clusters/lab/europe-west4/c1"
        - "clusters/staging/us-central1/c1"
        - "clusters/staging/us-central1/c2"
        - "clusters/staging/europe-west4/c1"
  - sourcePath: "clusters/staging/[^/]*/[^/]*" # This will start a promotion to prod from any "staging" path
    conditions:
      prHasLabels:
        - "quick_promotion" # This flow will run only if PR has "quick_promotion" label, see targetPaths below
    promotionPrs:
      - targetPaths:
        - "clusters/prod/us-west1/c2" # First PR for only a single cluster
      - targetPaths:
        - "clusters/prod/europe-west3/c2" # 2nd PR will sync all 4 remaining clusters
        - "clusters/prod/europe-west4/c2"
        - "clusters/prod/us-central1/c2"
        - "clusters/prod/us-east4/c2"
  - sourcePath: "clusters/staging/[^/]*/[^/]*" # This flow will run on PR without "quick_promotion" label
    promotionPrs:
      - targetPaths:
        - "clusters/prod/us-west1/c2" # Each cluster will have its own promotion PR
      - targetPaths:
        - "clusters/prod/europe-west3/c2"
      - targetPaths:
        - "clusters/prod/europe-west4/c2"
      - targetPaths:
        - "clusters/prod/us-central1/c2"
      - targetPaths:
        - "clusters/prod/us-east4/c2"
dryRunMode: true
autoApprovePromotionPrs: true
commentArgocdDiffonPR: true
autoMergeNoDiffPRs: true
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

## GitHub API Limit

Telefonistka doesn't use GitHub git protocol but only uses the REST and GraphQL APIs. This can make it a somewhat "heavy" user.
But in our experience a team of ~20 engineers pushing ~ 30 PRs a day didn't come close to depleting the Telefonistka app API quota.

Check [GitHub docs](https://docs.github.com/en/apps/creating-github-apps/creating-github-apps/rate-limits-for-github-apps) for details about the API rate limit.
This is the section relevant for GitHub Application style installation of Telefonistka:

> GitHub Apps making server-to-server requests use the installation's minimum rate limit of 5,000 requests per hour. If an application is installed on an > organization with more than 20 users, the application receives another 50 requests per hour for each user. Installations that have more than 20 repositories receive another 50 requests per hour for each repository. The maximum rate limit for an installation is 12,500 requests per hour.

Rate limit status is tracked by `telefonistka_github_github_rest_api_client_rate_limit`  and `telefonistka_github_github_rest_api_client_rate_remaining` [metrics](docs/observability.md#metrics)
