<!-- markdownlint-disable MD033 -->
<p align="center">  <img src="https://user-images.githubusercontent.com/1616153/217223509-9aa60a5c-a263-41a7-814d-a1bf2957acf6.png " width="150"> </p>

# <p align="center"> Telefonistka</p>

<!-- markdownlint-enable MD033 -->

Telefonistka is a Github webhook server/Bot that facilitate change promotion across environments/failure domains in Infrastructure as Code GitOps repos.

It assumes the [the repeatable part if your infrastucture is modeled in folders](#modeling-environmentsfailure-domains-in-an-iac-gitops-repo)

Based on configuration in the IaC repo, the bot will open Pull Requests that syncs components from "sourcePath"s to "targetPaths".

Providing reasonably flexible control over what is promoted to where and in what order.

## Modeling environments/failure-domains in an IaC GitOps repo

RY is the new DRY!

In GitOps IaC implementations, different environments(`dev`/`prod`/...) and failure domains(`us-east-1`/`us-west-1`/...) must be represented in distinct files, folders, Git branches or even repositories to allow gradual and controlled rollout of changes across said environments/failure domains.

At Wayfair's Kubernetes team we choose "The Folders" approach, more about this choice [here](docs/modeling_environments_in_gitops_repo.md).

Specifically, we choose the following scheme to represent all the Infrastructure components running in our Kubernetes clusters:
`clusters`/[environment]/[cloud region]/[cluster identifier]/[component name]
for  example:
`clusters/staging/us-central1/c2/prometheus/`
`clusters/staging/us-central1/c2/nginx-ingress/`
`clusters/prod/us-central1/c2/prometheus/`
`clusters/prod/us-central1/c2/nginx-ingress/`
`clusters/prod/europe-west4/c2/prometheus/`
`clusters/prod/europe-west4/c2/nginx-ingress/`

While this approach provide multiple benefits it does mean the user is expected to make changes in multiple files and folder in order to apply a single change to multiple environments/FDs.

Manually syncing those files is time consuming, error prone and generally not fun. And in the long run, undesired drift between those environments/FDs is almost guaranteed to accumulate as humans do that thing where they fail to be perfect at what they do.

This is where Telefonistka comes in.

Telefonistka will automagically create Pull Requests that "sync" our changes to the right folder or folders, enabling the usage of the familiar PR functionality to control promotions while avoiding the toil related to manually syncing directories and checking for environments/FDs drift.

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

### Control over grouping of targetPaths syncs in PRs

e.g. "Sync all dev clusters in one PR but open a dedicated PR for every production cluster"

### Optional in-component allow/block override list

e.g. "This component should not be deployed to production" or "Deploy this only in the us-east-4 region"

### Drift detection

warns user on "unsynced" environment on open PRs ("Staging the Production are not synced, these are the differences")
This is how this warnning looks in the PR:

<!-- markdownlint-disable MD033 -->
<img src="https://user-images.githubusercontent.com/1616153/219383563-8b833c17-7701-45b6-9471-d937d03142f4.png"  width="50%" height="50%">
<!-- markdownlint-enable MD033 -->

## Installation and Configuration

See [here](docs/installation.md)

## Observability

See [here](docs/observability.md)

## Development

* use Ngrok ( `ngrok http 8080` ) to expose the local instance
* See the URLs in ngrok command output.
* Add a webhook to repo setting (don't forget the `/webhook` path in the URL).
* Content type needs to be `application/json`, **currently** only PR events are needed

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
