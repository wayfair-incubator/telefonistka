# Modeling Distinct Environments/Failure Domains In Gitops Repo

## Terminology

`Environment(env)`: a distinct part of your Infrastructure that is used to run your services and cloud resources to support some goal. For example, `Production` is used to serve actual customers, `Staging` could be used to test how new version of services interact with the rest of the platfrom, and `Lab` is where new and untested changes are initial deployed to.
Having a production and at least one non-production environment is practically assumed.

`Failure domain(FD)`: These represent a repeatable part of your infrastructure that you *choose* to deploy in gradual steps to control the "blast radius" of a bad deploy/configuration. A classic example could be cloud regions like `us-east-1` or `us-west-1` for a company that runs a multi-region setup.
Smaller or younger companies might not have such distinct failure domains as the extra "safety" they provide might not be worth the investment.

In some cases I will use the term `envs` to refer to both Environments and Failure domains as from the perspective of the IaC tool, the GitOps pipeline/controller and Telefonistka they are the same.

## Methods

### Single instance 

All envs are controlled from a single file/folders, a single git commit change them all *at once*.
Even if you have per env/FD paramater override files(e.g. Helm value files/Terraform `.tfvars`), any change to the shared code that interacts with these files will be applied the all envs at once(GitOps!), somewhat negating their benefits 

### Branch per Env/FD

While using Git branches allows using git native tools for promoting changes(git merge) and inspecting drift(git diff) it quickly becomes cumbersome as the number of distinct environment/FDs grows. Additionally, syncing all your infrastructure from the main branch keeps the GitOps side of things more intuitive and make the promotion side more observable.

### Directory per Env/FD

This leaves us with "the multiple folders" approach, while gaining simplicity and observability, we are no longer able to use Git native tools to promote our changes across environments - it would requires us to manually copy files around, (r)sync directories or even worse - manually make the same change in multiple files.

### Git Repo per Env/FD



## External resources


https://codefresh.io/blog/stop-using-branches-deploying-different-gitops-environments/

https://codefresh.io/blog/how-to-model-your-gitops-environments-and-promote-releases-between-them/

https://www.sokube.io/en/blog/promoting-changes-and-releases-with-gitops
