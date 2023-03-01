## Metrics

|name|type|description|labels|
|---|---|---|---|
|telefonistka_github_github_operations_total|counter|"The total number of Github API operations|`api_group`, `api_path`, `repo_slug`, `status`, `method`|
|telefonistka_github_github_rest_api_client_rate_remaining|gauge|The number of remaining requests the client can make this hour||
|telefonistka_github_github_rest_api_client_rate_limit|gauge|The number of requests per hour the client is currently limited to||
|telefonistka_webhook_server_webhook_hits_total|counter|The total number of validated webhook hits|`parsing`|

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
```

