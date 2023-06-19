# GitHub Push events fanout/multiplexing

GitOps operators like ArgoCD can listen for GitHub webhooks to ensure short delays in their reconciliation loop.

But in some scenarios the number of needed webhooks endpoint exceed the maximum supported by GitHub(think 10 cluster each with in-cluster ArgoCD server and ArgoCD applicationSet controller).
Additionlly, configuring said webhooks manually is time consuming and error prone.

Telefonistka can forward these HTTP requests to multiple endpoint and can even filter or dynamically choose the endpoint URL based on the file changed in the Commit.
Assuming Telefonistka is deployed as a GitHub Application, this also ease the setup process as the webhook setting(event types, URL, secret) is already a part of the application configuration.


This configuration exmple will forward github push events that include changes in `workspace/` dir to the lab argocd server and  applicationset controllers webhook servers and will forward event  that touchs `clusters/`to URLs generated with regex, base of first 3 directiry elements after `clusters/`


```yaml
webhookEndpointRegexs:
  - expression: "^workspace\/[^/]*\/.*"
    replacements:
      - "https://lab-argocd-server.example.com/webhook"
      - "https://lab-argocd-applicationset.example.com/webhook"

  - expression: "^clusters\/([^/]*)\/([^/]*)\/([^/]*)\/.*"
    replacements:
      - "https://${1}-${2}-${3}-argocd-server.example.com/webhook"
      - "https://${1}-${2}-${3}-argocd-applicationset.example.com/webhook"

```

Telefonistka checks the regex per each file affected by a commit, but stops after the first expression match(per file).

So ordering of the `webhookEndpointRegexs` elements is significant.


This simpeller configuration will and push event to 7 hardcoded servers

```yaml
webhookEndpointRegexs:
  - expression: "^.*$"
    replacements:
      - "https://argocd-server1.example.com/webhook"
      - "https://argocd-server2.example.com/webhook"
      - "https://argocd-server3.example.com/webhook"
      - "https://argocd-server4.example.com/webhook"
      - "https://argocd-server5.example.com/webhook"
      - "https://argocd-server6.example.com/webhook"
      - "https://argocd-server6.example.com/webhook"
```
