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

Regardless of the tool you use to describe your infrastructure, or if your IaC repo includes code or just references to some versioned artifacts like helm charts/TF modules, you need a way to control how changes are made across environments(`dev`/`prod`/...) and failure domains(`us-east-1`/`us-west-1`/...).

If changes are applied immediately when they are committed to the repo, this means these  environments and failure domains need to be represented as different folders or branches to provide said control.

While using Git branches allows using git native tools for promoting changes(git merge) and inspecting drift(git diff) it quickly becomes cumbersome as the number of distinct environment/FDs grows. Additionally, syncing all your infrastructure from the main branch keeps the GitOps side of things more intuitive and make the promotion side more observable.

This leaves us with "the folders" approach, while gaining simplicity and observability it would requires us to manually copy files around, (r)sync directories or even worse - manually make the same change in multiple files.

This is where Telefonistka comes in.

## Notable Features

### IaC stack agnostic
Terraform, Helmfile, ArgoCD whatever, as long as environments and sites are modeled as folders and components are copied between environments "as is".
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

<img src="https://user-images.githubusercontent.com/1616153/219384172-27b960a8-afd1-42d1-8d5b-4b802134b851.png"  width="50%" height="50%">

### Control over grouping of targetPaths syncs in PRs
e.g. "Sync all dev clusters in one PR but open a dedicated PR for every production cluster"
### Optional in-component allow/block override list
e.g. "This component should not be deployed to production" or "Deploy this only in the us-east-4 region"
### Drift detection
warns user on "unsynced" environment on open PRs ("Staging the Production are not synced, these are the differences")
This is how this warnning looks in the PR:

<img src="https://user-images.githubusercontent.com/1616153/219383563-8b833c17-7701-45b6-9471-d937d03142f4.png"  width="50%" height="50%">

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
