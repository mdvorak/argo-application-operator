# csas-application-operator

K8s operator that generates `Application.argocd.io` objects from `Application.ops.csas.cz` templates.
Point is to allow namespace administrators create ArgoCD applications on their own, without need to modify ArgoCD
system namespace directly.

## Deployment

TODO

### Operations

TODO status
TODO metrics, logging

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

Run `TARGET_NAMESPACE=<yourns> operator-sdk run --local --namespace <sourcens>`

Note that when another operator instance runs in the cluster, additional unnecessary reconciliation loops might be 
triggered, and status will be updated by both. But an operator should produce expected output anyway.
