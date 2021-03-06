# csas-application-operator

REPOSITORY MOVED TO https://github.com/cs-poc/csas-application-operator

K8s operator that generates `Application.argocd.io` objects from `Application.ops.csas.cz` templates.
Point is to allow namespace administrators create ArgoCD applications on their own, without need to modify ArgoCD
system namespace directly.

## Description

During standard operation, operator watches all namespaces in a cluster for objects `Application.ops.csas.cz`.
They share `spec.source` configuration, which is copied to a argo namespace into `Application.argocd.io` objects. It handles
properly setting of a project, destination etc.

For example, following object created in namespace `foo`
```yaml
apiVersion: ops.csas.cz/v1alpha1
kind: Application
metadata:
  name: guestbook
  namespace: foo
spec:
  source:
    path: guestbook
    repoURL: 'https://github.com/argoproj/argocd-example-apps'
```

creates following in namespace `argo`
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  labels:
    app.kubernetes.io/managed-by: csas-application-operator
    application.ops.csas.cz/owner-api-group: ops.csas.cz
    application.ops.csas.cz/owner-api-version: v1alpha1
    application.ops.csas.cz/owner-kind: Application
    application.ops.csas.cz/owner-name: mdvorak-example
    application.ops.csas.cz/owner-namespace: mdvorak
  name: foo-guestbook
  namespace: argo
spec:
  destination:
    namespace: foo
    server: 'https://kubernetes.default.svc'
  project: foo
  source:
    path: guestbook
    repoURL: 'https://github.com/argoproj/argocd-example-apps'
```

Note that in order to avoid name conflicts, namespace is added as prefix into application name, that is `guestbook`
is transformed into `foo-guestbook`. If the name would already contain prefix, it wouldn't be duplicated.

## Deployment

TODO

### Operations

Operator logs in JSON format into stdout, which means logs are available in standard cluster logging solution (Elastic).

It also exposes metrics endpoint, see 
[operator sdk metrics readme](https://github.com/operator-framework/operator-sdk/blob/master/doc/user/metrics/README.md).

In addition, every `Application.ops.csas.cz` have status updated with conditions (either success or error message), and
list of created ArgoCD Application objects. See example

```yaml
apiVersion: ops.csas.cz/v1alpha1
kind: Application
metadata:
  finalizers:
    - finalizer.application.ops.csas.cz
  name: guestbook
  namespace: foo
spec:
  source:
    path: guestbook
    repoURL: 'https://github.com/argoproj/argocd-example-apps'
status:
  conditions:
    - lastTransitionTime: '2020-03-25T10:55:06Z'
      message: reconciliation successful
      reason: Created
      status: 'True'
      type: Available
  references:
    - apiVersion: argoproj.io/v1alpha1
      kind: Application
      name: foo-guestbook
      namespace: argo
```

## Development

Standard [operator sdk user guide](https://github.com/operator-framework/operator-sdk/blob/master/doc/user-guide.md)
applies, read it first.

### Project Structure

* `build/` - image Dockerfile and additional content
* `cmd/manager/` - operator main method, also all schemas are registered there
* `deploy/` - kubernetes manifest needed for deployment
* `deploy/crds/` - automatically generated CRD from go struct definitions (call `operator-sdk generate crds`)
* `pkg/` - operator APIs and controllers
* `version/` - version string
* `go.mod` - project dependencies, managed both manually and automatically

### Building

Run `operator-sdk build <image>:<tag>` and `docker push <image>:<tag>` to publish it (can be replaced by buildah or podman).

### Local Testing

Run `ARGO_NAMESPACE=<yourns> operator-sdk run --local --namespace <sourcens>`

Note that when another operator instance runs in the cluster, additional unnecessary reconciliation loops might be 
triggered, and status will be updated by both. But an operator should produce expected output anyway.
