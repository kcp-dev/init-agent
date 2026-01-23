# Installing the Init Agent

This page describes the necessary steps to setup the Init Agent for an existing [kcp][kcp] installation.

## Prerequisites

- A running kcp installation.
- A kubeconfig for kcp with the appropriate permissions to access its resources and the
  `initializingworkspaces` virtual workspace.

## WorkspaceTypes

The basic [mode of operation](README.md#mode-of-operation) of the init-agent relies on kcp's
initializers feature. For this feature, newly created workspaces are made inaccessible for users
until all initializers (like finalizers, but in reverse) have been removed from the `LogicalCluster`
that backs a workspace. Initializers are the cluster name + name of the `WorkspaceType`, for example
`root:my-type` or `dhkgfj2gvrhbf:test-env`.

!!! warning
    Since each `WorkspaceType` has exactly one (optional) initializer name, and it can only be
    removed once from a `LogicalCluster`, it's critical that you use dedicated workspace types for
    every bootstrapping purpose.

    This means there can only be exactly one `InitTarget` in the entire kcp installation that refers
    to a `WorkspaceType`. And only a single init-agent may process each `InitTarget`.

    **Do not** use the init-agent with kcp's own `WorkspaceTypes`, as this could interfere with
    kcp's core functionality.

    You can make use of `WorkspaceTypes` extending each other to combine more complex bootstrapping
    behaviour.

The very first step to using the init-agent is to make sure you have your own, dedicated
`WorkspaceTypes`. These can exist anywhere (any clusters) inside your kcp installation. It's important
to enable `spec.initializer`, otherwise kcp will not add the type's initializer to newly created
clusters.

Let's create a sample type in `root:ws-types`:

```yaml
apiVersion: tenancy.kcp.io/v1alpha1
kind: WorkspaceType
metadata:
  name: dev-environment
spec:
  # important, this must be set to true
  initializer: true

  defaultChildWorkspaceType:
    name: universal
    path: root

  # extend the universal type to gain default kcp workspace behaviour
  extend:
    with:
      - name: universal
        path: root
```

Suppose your workspace `root:ws-types` is cluster `8924zrg2i5g4dr`, then this `WorkspaceType`'s
initializer will be `8924zrg2i5g4dr:dev-environment`. The init-agent will automatically figure this
out for you though.

## InitTargets

Now that the `WorkspaceTypes` are ready, it's time to configure them for the init-agent. To do so,
we create `InitTarget` objects, which connect a type with a number of "init source". An init source
is anything that provides manifests of Kubernetes objects.

There can be up to one `InitTarget` in the entire kcp installation for any given `WorkspaceType`. The
target and the type can be in different workspaces, however you must keep all `InitTargets` that
should be processed by one init-agent in one workspace.

Let's create a (dummy) `InitTarget` in `root:init-agent`:

```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTarget
metadata:
  name: init-dev-environment
spec:
  # reference the WorkspaceType to bootstrap
  workspaceTypeRef:
    path: root:ws-types
    name: dev-environment

  # list all the manifest sources
  sources: []
```

## Init Sources

Each `InitTarget` contains a list of init sources, which in turn are anything can provides a
number of Kubernetes objects (usually in the form of YAML manifests). The mechanism is purposefully
extensible, though at the moment only a limited number of such sources is implemented.

### InitTemplate

This is the most basic init source. Init templates are Kubernetes objects that simply contain a
single Go template string that is executed for each new cluster to be bootstrapped. This is very
similar to how Helm chart templates work.

`InitTemplate` objects must reside in the same cluster as the `InitTargets` that are referring to
them.

Let's create a simple one, again in `root:ws-types`, that creates a namespace and then places a
`ConfigMap` into it:

{% raw %}
```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTemplate
metadata:
  name: cluster-info-configmap
spec:
  template: |
    apiVersion: v1
    kind: ConfigMap
    metadata:
      namespace: cluster-info
      name: info
    data:
      cluster: "{{ .ClusterName }}"
      workspace: "{{ .ClusterPath }}"

    ---
    apiVersion: v1
    kind: Namespace
    metadata:
      name: cluster-info
```
{% endraw %}

Note how the namespace is listed after the `ConfigMap`. This is to demonstrate that after executing
the template, the resulting objects are sorted by their hierarchy to ensure, for example, that CRDs
are created before objects using them.

Then update the `InitTarget` to refer to this template:

```yaml
apiVersion: initialization.kcp.io/v1alpha1
kind: InitTarget
metadata:
  name: init-dev-environment
spec:
  #...

  sources:
    - template:
        name: cluster-info-configmap
```

## Running the Agent

For regular use, the init-agent should be installed using its [Helm chart][helmchart], but for
debugging purposes it's also possible to run the agent locally.

### Helm Chart

To use the Helm chart, first add the repository to your local system:

```bash
helm repo add kcp https://kcp-dev.github.io/helm-charts
helm repo update
```

At the very least you will have to provide

* a kubeconfig to access kcp
* the name of the workspace/cluster where the `InitTargets` and other resources
  reside

Put both in your `myvalues.yaml` (check the `values.yaml` for more examples).

You can now install the chart:

```bash
helm upgrade --install my-init-agent kcp/init-agent --values ./myvalues.yaml
```

### Locally

For development purposes you can run the agent directly. You can build your own from
source or download one of the [ready-made releases][releases]. To run it, you need
to provide the kcp kubeconfig and the workspace/cluster where the `InitTargets`
reside:

```bash
./init-agent \
  --kubeconfig /path/to/kcp.kubeconfig \
  --config-workspace root:my-org:init-agent
```

[kcp]: https://kcp.io
[helmchart]: https://github.com/kcp-dev/helm-charts/tree/main/charts/init-agent
[releases]: https://github.com/kcp-dev/init-agent/releases
