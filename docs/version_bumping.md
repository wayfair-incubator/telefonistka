# Version Bumping

If your IaC repo deploys software you maintain internally you probably want to automate artifact version bumping.
Telefonistka can automate opening the IaC repo PR for the version change from the  Code repo pipeline.

Currently, three modes of operation are supported:

## Whole file overwrite

```shell
Bump artifact version based on provided file content.
This open a pull request in the target repo.

Usage:
  telefonistka bump-overwrite [flags]

Flags:
      --auto-merge                            Automatically merges the created PR, defaults to false.
  -c, --file string                           File that holds the content the target file will be overwritten with, like "version.yaml" or '<(echo -e "image:\n  tag: ${VERSION}")'.
  -g, --github-host string                    GitHub instance HOSTNAME, defaults to "github.com". This is used for GitHub Enterprise Server instances.
  -h, --help                                  help for bump-overwrite.
  -f, --target-file string                    Target file path(from repo root), defaults to TARGET_FILE env var.
  -t, --target-repo string                    Target Git repository slug(e.g. org-name/repo-name), defaults to TARGET_REPO env var.
  -a, --triggering-actor string               GitHub user of the person/bot who triggered the bump, defaults to GITHUB_ACTOR env var.
  -p, --triggering-repo octocat/Hello-World   Github repo triggering the version bump(e.g. octocat/Hello-World) defaults to GITHUB_REPOSITORY env var.
  -s, --triggering-repo-sha string            Git SHA of triggering repo, defaults to GITHUB_SHA env var.
```

notes:

* This can create new files in the target repo.
* This was intended for cases where the IaC configuration allows adding additional minimal parameter/values file that only includes version information.

## Regex based search and replace

```shell
Bump artifact version in a file using regex.
This open a pull request in the target repo.

Usage:
  telefonistka bump-regex [flags]

Flags:
      --auto-merge                            Automatically merges the created PR, defaults to false.
  -g, --github-host string                    GitHub instance HOSTNAME, defaults to "github.com". This is used for GitHub Enterprise Server instances.
  -h, --help                                  help for bump-regex.
  -r, --regex-string string                   Regex used to replace artifact version, e.g. 'tag:\s*(\S*)',
  -n, --replacement-string string             Replacement string that includes the version of new artifact, e.g. 'tag: v2.7.1'.
  -f, --target-file string                    Target file path(from repo root), defaults to TARGET_FILE env var.
  -t, --target-repo string                    Target Git repository slug(e.g. org-name/repo-name), defaults to TARGET_REPO env var.
  -a, --triggering-actor string               GitHub user of the person/bot who triggered the bump, defaults to GITHUB_ACTOR env var.
  -p, --triggering-repo octocat/Hello-World   Github repo triggering the version bump(e.g. octocat/Hello-World) defaults to GITHUB_REPOSITORY env var.
  -s, --triggering-repo-sha string            Git SHA of triggering repo, defaults to GITHUB_SHA env var.
```

notes:

* This assumes the target file already exist in the target repo.

## YAML based value replace

```shell
Bump artifact version in a file using yaml selector.
This will open a pull request in the target repo.
This command uses yq selector to find the yaml value to replace.

Usage:
  telefonistka bump-yaml [flags]

Flags:
      --address string                        Yaml value address described as a yq selector, e.g. '.db.[] | select(.name == "postgres").image.tag'.
      --auto-merge                            Automatically merges the created PR, defaults to false.
  -g, --github-host string                    GitHub instance HOSTNAME, defaults to "github.com". This is used for GitHub Enterprise Server instances.
  -h, --help                                  help for bump-yaml
  -n, --replacement-string string             Replacement string that includes the version value of new artifact, e.g. 'v2.7.1'.
  -f, --target-file string                    Target file path(from repo root), defaults to TARGET_FILE env var.
  -t, --target-repo string                    Target Git repository slug(e.g. org-name/repo-name), defaults to TARGET_REPO env var.
  -a, --triggering-actor string               GitHub user of the person/bot who triggered the bump, defaults to GITHUB_ACTOR env var.
  -p, --triggering-repo octocat/Hello-World   Github repo triggering the version bump(e.g. octocat/Hello-World) defaults to GITHUB_REPOSITORY env var.
  -s, --triggering-repo-sha string            Git SHA of triggering repo, defaults to GITHUB_SHA env var.
```

notes:

* This assumes the target file already exist in the target repo.
