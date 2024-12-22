```diff
diff live target
--- live
+++ target
@@ -21,14 +21,14 @@
     name: example-baz-bar
     namespace: gitops-demo-ns2
     resourceVersion: "2168525640"
     uid: f845fd72-d6d9-48f2-b0f2-2def6807deb8
   rbacBindings:
   - clusterRoleBindings:
     - clusterRole: view
     name: security-audit-viewer-vault
     subjects:
     - kind: Group
-      name: vault:some-team@domain.tld
+      name: vault:some-team-name@domain.tld
   spec:
     deploymentName: example-baz-bar
-    replicas: 63
+    replicas: 42
```
