# Waiting for Readiness

In some scenarios, simply creating objects during initialization is not enough. You may need to wait for
certain resources to become ready before the init-agent marks the workspace as initialized. For example,
a CRD must be `Established` before custom resources using it can be created by other initializers
further down the chain.

The `initialization.kcp.io/wait-for-ready` annotation allows you to express this requirement on
individual manifests.

## How It Works

When the init-agent encounters a manifest (from any of its [init sources](./init-sources/)) with
the `initialization.kcp.io/wait-for-ready` annotation, it will:

1. Create (or confirm the existence of) the object as usual.
2. Re-fetch the object's current state from the API server.
3. Check whether the condition type specified in the annotation's value has `status: "True"` in the
   object's `status.conditions` list.
4. If the condition is not yet `True`, the agent requeues reconciliation and tries again after a few
   seconds.
5. Only once **all** annotated objects across **all** sources have their required conditions met will
   the agent remove the initializer from the workspace, completing initialization.

The annotation value must be the **name of a condition type** (e.g. `Established`, `Ready`,
`Available`). This condition must appear in the standard Kubernetes `status.conditions` array of
the resource.

!!! warning "Important"
    Waiting for a condition to become `True` inherently means that **some process must set that
    condition**. In some cases this happens automatically (e.g. the Kubernetes API server sets
    `Established` on CRDs), but in other cases you may need a dedicated controller or operator to
    act on the resource and update its status.

    Due to the nature of kcp's workspace initialization, the workspace is not accessible through
    the regular API while it still has initializers. Only processes that work through the same
    `initializingworkspaces` virtual workspace – i.e. processes that are registered for the **same
    initializer** as the init-agent – can see and modify objects in the workspace during
    initialization.

    This means that if you need an external controller to make a resource "ready", that controller
    must also operate on the same initializer's `initializingworkspaces` view. Without this, the
    controller will not be able to access the workspace and therefore cannot set the condition the
    init-agent is waiting for. The initialization would be stuck indefinitely.

    In practice, this is most relevant for custom operators that need to reconcile resources created
    by the init-agent. Make sure these operators have the appropriate kcp permissions and are
    configured to watch the same initializing workspaces.

## Usage

Add the annotation to any manifest inside an `InitTemplate`'s `spec.template`. The following example
creates a CRD and waits for it to become `Established` before initialization is considered complete:

```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTemplate
metadata:
  name: widgets-crd
spec:
  template: |
    apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata:
      name: widgets.example.com
      annotations:
        initialization.kcp.io/wait-for-ready: "Established"
    spec:
      group: example.com
      names:
        kind: Widget
        listKind: WidgetList
        plural: widgets
        singular: widget
      scope: Cluster
      versions:
        - name: v1alpha1
          served: true
          storage: true
          schema:
            openAPIV3Schema:
              type: object
```

In this example the init-agent will create the CRD and then wait until its `Established` condition
is `True` before considering this source complete. CRDs are just a nice example of a Kube-native
resource that on its own becomes ready.

The agent will keep retrying indefinitely. If a condition is never set, the workspace will remain
in the initializing state. Use kcp's workspace lifecycle management to handle stuck workspaces if
necessary.

The annotation works with **any Kubernetes resource** that follows the standard conditions pattern
in its status. Common examples include:

| Resource | Typical Condition |
| -------- | ----------------- |
| `CustomResourceDefinition` | `Established` |
| `Deployment` | `Available` |
| `APIBinding` (kcp) | `Ready` |
