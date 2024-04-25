<!-- markdownlint-disable MD033 -->
<p align="center">  <img src="https://user-images.githubusercontent.com/1616153/217223509-9aa60a5c-a263-41a7-814d-a1bf2957acf6.png " width="150"> </p>

# <p align="center"> Telefonistka</p>

<!-- markdownlint-enable MD033 -->

Telefonistka is a Github webhook server/Bot that facilitates change promotion across environments/failure domains in Infrastructure as Code(IaC) GitOps repos.

It assumes the [repeatable part of your infrastucture is modeled in folders](#modeling-environmentsfailure-domains-in-an-iac-gitops-repo)

Based on configuration in the IaC repo, the bot will open pull requests that sync components from "sourcePaths" to "targetPaths".

Providing reasonably flexible control over what is promoted to where and in what order.

A 10 minutes ArgoCon EU 2023 session describing the project:

[![ArgoCon EU 2023 session](https://img.youtube.com/vi/oiSsSiROj10/0.jpg)](https://www.youtube.com/watch?v=oiSsSiROj10)

## Modeling environments/failure-domains in an IaC GitOps repo

RY is the new DRY!

In GitOps IaC implementations, different environments(`dev`/`prod`/...) and failure domains(`us-east-1`/`us-west-1`/...) must be represented in distinct files, folders, Git branches or even repositories to allow gradual and controlled rollout of changes across said environments/failure domains.

At Wayfair's Kubernetes team we choose the "folders" approach, more about other choices [here](docs/modeling_environments_in_gitops_repo.md).

Specifically, we choose the following scheme to represent all the Infrastructure components running in our Kubernetes clusters:
`clusters`/`[environment]`/`[cloud region]`/`[cluster identifier]`/`[component name]`

for  example:

```text
clusters/staging/us-central1/c2/prometheus/
clusters/staging/us-central1/c2/nginx-ingress/
clusters/prod/us-central1/c2/prometheus/
clusters/prod/us-central1/c2/nginx-ingress/
clusters/prod/europe-west4/c2/prometheus/
clusters/prod/europe-west4/c2/nginx-ingress/
```

While this approach provides multiple benefits it does mean the user is expected to make changes in multiple files and folders in order to apply a single change to multiple environments/FDs.

Manually syncing those files is time consuming, error prone and generally not fun. And in the long run, undesired drift between those environments/FDs is almost guaranteed to accumulate as humans do that thing where they fail to be perfect at what they do.

This is where Telefonistka comes in.

Telefonistka will automagically create pull requests that "sync" our changes to the right folder or folders, enabling the usage of the familiar PR functionality to control promotions while avoiding the toil related to manually syncing directories and checking for environments/FDs drift.

## Notable Features

### IaC stack agnostic

Terraform, Helmfile, ArgoCD whatever, as long as environments and sites are modeled as folders and components are copied between environments "as is".

### Unopinionated directory structure

The [in-configuration file](docs/installation.md#repo-configuration) is flexible and even has some regex support.

The project goal is support any reasonable setup and we'll try to address unsupported setups.

### Multi stage promotion schemes

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

Telefonistka annotates the PR with the historic "flow" of the promotion:

<!-- markdownlint-disable MD033 -->
<img src="https://user-images.githubusercontent.com/1616153/219384172-27b960a8-afd1-42d1-8d5b-4b802134b851.png"  width="50%" height="50%">
<!-- markdownlint-enable MD033 -->

### Control granularity of promotion PRs

Allows separating promotions into a separate PRs per environment/failure domain or group some/all of them.

e.g. "Sync all dev clusters in one PR but open a dedicated PR for every production cluster"

Also allows automatic merging of PRs based on the promotion policy.

e.g. "Automatically merge PRs that promote to multiple `lab` environments"

### Optional per-component allow/block override list

Allows overriding the general(per-repo) promotion policy on a per component level.

e.g. "This component should not be deployed to production" or "Promote this only to the us-east-4 region"

### Drift detection and warning

Warns user on [drift between environment/failure domains](docs/modeling_environments_in_gitops_repo.md#terminology) on open PRs ("Staging and Production are not synced, these are the differences")
This is how this warning looks in the PR:

<!-- markdownlint-disable MD033 -->
<img src="https://user-images.githubusercontent.com/1616153/219383563-8b833c17-7701-45b6-9471-d937d03142f4.png"  width="50%" height="50%">
<!-- markdownlint-enable MD033 -->

### ArgoCD integration

Telefonistka can compare manifests in PR branches to live objects in the clusters and comment on the difference in PRs

<!-- markdownlint-disable MD033 -->
<img width="50%" alt="image" src="https://github.com/commercetools/telefonistka/assets/1616153/fe38dc7a-2dea-4461-a0bf-3b07531135a9">
<!-- markdownlint-enable MD033 -->

### Artifact version bumping from CLI

If your IaC repo deploys software you maintain internally you probably want to automate artifact version bumping.
Telefonistka can automate opening the IaC repo PR for the version change from the  Code repo pipeline:

```shell
telefonistka bump-overwrite \
    --target-repo Oded-B/telefonistka-example \
    --target-file workspace/nginx/values-version.yaml \
    --file <(echo -e "image:\n  tag: v3.4.9") \
```

It currently supports full file overwrite, regex and yaml based replacement.
See [here](docs/version_bumping.md) for more details

### GitHub Push events fanout/multiplexing

Some GitOps operators can listen for GitHub webhooks to ensure short delays in the reconciliation loop.

But in some scenarios the number of needed webhooks endpoint exceed the maximum supported by GitHub(think 10 cluster each with in-cluster ArgoCD server and ArgoCD applicationSet controller).

Telefonistka can forward these HTTP requests to multiple endpoint and can even filter or dynamically choose the endpoint URL based on the file changed in the Commit.

This example configuration includes regex bases endpoint URL generation:

```yaml
webhookEndpointRegexs:
  - expression: "^workspace/[^/]*/.*"
    replacements:
      - "https://kube-argocd-c1.service.lab.example.com/api/webhoook"
      - "https://kube-argocd-applicationset-c1.service.lab.example.com/api/webhoook"
      - "https://example.com"
  - expression: "^clusters/([^/]*)/([^/]*)/([^/]*)/.*"
    replacements:
      - "https://kube-argocd-${3}.${1}.service.{2}.example.com/api/webhoook"
      - "https://kube-argocd-applicationset-${2}.service.${1}.example.com/api/webhoook"

```

see [here](docs/webhook_multiplexing.md) for more details

## Installation and Configuration

See [here](docs/installation.md)

## Observability

See [here](docs/observability.md)

## Development

* use Ngrok ( `ngrok http 8080` ) to expose the local instance
* See the URLs in ngrok command output.
* Add a webhook to repo setting (don't forget the `/webhook` path in the URL).
* Content type needs to be `application/json`, **currently** only PR events are needed

To publish container images from a forked repo set the `IMAGE_NAME` and `REGISTRY` GitHub Action Repository variables to use GitHub packages.
`REGISTRY` should be `ghcr.io` and `IMAGE_NAME` should match the repository slug, like so:
like so:
<!-- markdownlint-disable MD033 -->
<img width="785" alt="image" src="https://github.com/commercetools/telefonistka/assets/1616153/2f7201d6-fdb2-4cbf-8705-d6da7f4f6e80">
<!-- markdownlint-enable MD033 -->

## Roadmap

See the [open issues](https://github.com/wayfair-incubator/telefonistka/issues) for a list of proposed features (and known issues).

## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**. For detailed contributing guidelines, please see [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.
