## Metrics

|name|type|description|labels|
|---|---|---|---|
|telefonistka_github_github_operations_total|counter|"The total number of Github API operations|`api_group`, `api_path`, `repo_slug`, `status`, `method`|
|telefonistka_github_github_rest_api_client_rate_remaining|gauge|The number of remaining requests the client can make this hour||
|telefonistka_github_github_rest_api_client_rate_limit|gauge|The number of requests per hour the client is currently limited to||
|telefonistka_webhook_server_webhook_hits_total|counter|The total number of validated webhook hits|`parsing`|
|telefonistka_github_open_prs|gauge|The number of open PRs|`repo_slug`|
|telefonistka_github_open_promotion_prs|gauge|The number of open promotion PRs|`repo_slug`|
|telefonistka_github_open_prs_with_pending_telefonistka_checks|gauge|The number of open PRs with pending Telefonistka checks(excluding PRs with very recent commits)|`repo_slug`|
|telefonistka_github_commit_status_updates_total|counter|The total number of commit status updates, and their status (success/pending/failure)|`repo_slug`, `status`|

> [!NOTE]  
> telefonistka_github_*_prs metrics are only supported on installtions that uses GitHub App authentication as it provides an easy way to query the relevant GH repos.

Example metrics snippet:

```text
# HELP telefonistka_github_github_operations_total The total number of Github operations
# TYPE telefonistka_github_github_operations_total counter
telefonistka_github_github_operations_total{api_group="repos",api_path="",method="GET",repo_slug="Oded-B/telefonistka-example",status="200"} 8
telefonistka_github_github_operations_total{api_group="repos",api_path="contents",method="GET",repo_slug="Oded-B/telefonistka-example",status="200"} 76
telefonistka_github_github_operations_total{api_group="repos",api_path="contents",method="GET",repo_slug="Oded-B/telefonistka-example",status="404"} 13
telefonistka_github_github_operations_total{api_group="repos",api_path="issues",method="POST",repo_slug="Oded-B/telefonistka-example",status="201"} 3
telefonistka_github_github_operations_total{api_group="repos",api_path="pulls",method="GET",repo_slug="Oded-B/telefonistka-example",status="200"} 8
# HELP telefonistka_github_github_rest_api_client_rate_limit The number of requests per hour the client is currently limited to
# TYPE telefonistka_github_github_rest_api_client_rate_limit gauge
telefonistka_github_github_rest_api_client_rate_limit 100000
# HELP telefonistka_github_github_rest_api_client_rate_remaining The number of remaining requests the client can make this hour
# TYPE telefonistka_github_github_rest_api_client_rate_remaining gauge
telefonistka_github_github_rest_api_client_rate_remaining 99668
# HELP telefonistka_webhook_server_webhook_hits_total The total number of validated webhook hits
# TYPE telefonistka_webhook_server_webhook_hits_total counter
telefonistka_webhook_server_webhook_hits_total{parsing="successful"} 8
# HELP telefonistka_github_commit_status_updates_total The total number of commit status updates, and their status (success/pending/failure)
# TYPE telefonistka_github_commit_status_updates_total counter
telefonistka_github_commit_status_updates_total{repo_slug="foo/bar2",status="error"} 1
telefonistka_github_commit_status_updates_total{repo_slug="foo/bar2",status="pending"} 1
# HELP telefonistka_github_open_promotion_prs The total number of open PRs with promotion label
# TYPE telefonistka_github_open_promotion_prs gauge
telefonistka_github_open_promotion_prs{repo_slug="foo/bar1"} 0
telefonistka_github_open_promotion_prs{repo_slug="foo/bar2"} 10
# HELP telefonistka_github_open_prs The total number of open PRs
# TYPE telefonistka_github_open_prs gauge
telefonistka_github_open_prs{repo_slug="foo/bar1"} 0
telefonistka_github_open_prs{repo_slug="foo/bar2"} 21
# HELP telefonistka_github_open_prs_with_pending_telefonistka_checks The total number of open PRs with pending Telefonistka checks(excluding PRs with very recent commits)
# TYPE telefonistka_github_open_prs_with_pending_telefonistka_checks gauge
telefonistka_github_open_prs_with_pending_telefonistka_checks{repo_slug="foo/bar1"} 0
telefonistka_github_open_prs_with_pending_telefonistka_checks{repo_slug="foo/bar2"} 0
```
