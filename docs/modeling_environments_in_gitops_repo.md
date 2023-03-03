# Modeling Distinct Environments/Failure Domains In Gitops Repo

This document give a short overview on the available methods to model multiple environments/failure domains in a GitOps IaC repo.
More thorough articles can be found in the  [external resources section](#external-resources).

## Terminology

`Environment(env)`: a distinct part of your Infrastructure that is used to run your services and cloud resources to support some goal. For example, `Production` is used to serve actual customers, `Staging` could be used to test how new version of services interact with the rest of the platform, and `Lab` is where new and untested changes are initial deployed to.
Having a production and at least one non-production environment is practically assumed.

`Failure domain(FD)`: These represent a repeatable part of your infrastructure that you *choose* to deploy in gradual steps to control the "blast radius" of a bad deploy/configuration. A classic example could be cloud regions like `us-east-1` or `us-west-1` for a company that runs a multi-region setup.
Smaller or younger companies might not have such distinct failure domains as the extra "safety" they provide might not be worth the investment.

In some cases I will use the term `envs` to refer to both Environments and Failure domains as from the perspective of the IaC tool, the GitOps pipeline/controller and Telefonistka they are the same.

`Drift`: in this context, drift describes an unwanted/unintended difference between environment/FDs that is present in the Git state, for example a change that was made to the `Staging` environment but wasn't promoted to `Prod`

## Available Methods

### Single instance

All envs are controlled from a single file/folders, a single git commit change them all *at once*.
Even if you have per env/FD parameter override files(e.g. Helm value files/Terraform `.tfvars`), any change to the shared code(or a reference to a versioned artifact hosting the code) will be applied the all envs at once(GitOps!), somewhat negating the benefits of maintaining multiple envs.

### Git Branch per Env/FD

This allows using git native tools for promoting changes(`git merge`) and inspecting drift(`git diff`) but it quickly becomes cumbersome as the number of distinct environment/FDs grows. Additionally, syncing all your infrastructure from the main branch keeps the GitOps side of things more intuitive and make the promotion side more observable.

### Directory per Env/FD

This is our chosen approach and what Telefonistka currently supports.

See [section in README.md](../README.md#modeling-environmentsfailure-domains-in-an-iac-gitops-repo)

### Git Repo per Env/FD

This is the most complex but flexible solution, providing the strongest isolation in permission and policy enforcement.
This feels a bit too much considering the added complexity, especially if the number of envs is high or dynamic.
Telefonistka doesn't support this model currently.

## External resources

[Stop Using Branches for Deploying to Different GitOps Environments](https://codefresh.io/blog/stop-using-branches-deploying-different-gitops-environments/)

[How to Model Your Gitops Environments and Promote Releases between Them](https://codefresh.io/blog/how-to-model-your-gitops-environments-and-promote-releases-between-them/)

[Promoting changes and releases with GitOps](https://www.sokube.io/en/blog/promoting-changes-and-releases-with-gitops)
