# Documentation

The kcp init-agent is a Kubernetes agent for initializing newly created workspaces by applying any
number of Kubernetes objects in it. This can be used to ensure new clusters are automatically
bootstrapped for certain purposes.

Behind the scenes the init-agent uses kcp's `initializingworkspaces` virtual workspace to access new
clusters before they have become ready (regular access to initializing clusters is forbidden in kcp).
Once it has finished bootstrapping them, it removes an "initializer" from the cluster to signal to
kcp that the cluster can become ready.

The agent can source manifests for the objects to create from a variety of sources in theory, though
currently only `InitTemplate` objects are implemented.

## High-level Overview

To setup the agent, an administrator follows roughly these steps:

1. Create a dedicated `WorkspaceType` for which the workspaces should be initialized. This new
   `WorkspaceType` must have `spec.initializer` enabled to ensure kcp marks new workspaces as
   not-yet-ready.
2. Create an `InitTarget` object that connects the `WorkspaceType` with the init sources (as of now,
   a list of `InitTemplate`s).
3. Create a number of `InitTemplate` objects that contain Kubernetes manifests with Go templating
   (similar to how Helm templates work).
4. Run the init-agent by providing it a kubeconfig to access kcp and the name of the workspace where
   the `InitTarget` objects reside.

This concludes the initial setup. When users now create workspaces using the type created in step 1,
the init-agent will perform the desired bootstrapping.

## Mode of Operation

The init-agent will continuously watch `InitTarget` objects in the `--config-workspace` it was started
with. Each of these targets references exactly one `WorkspaceType` (there must be no overlaps, i.e.
no two `InitTarget`s must point to the same `WorkspaceType`). One single init-agent instance can
process many `InitTarget`s (optionally filtered by a label selector configured on the command line).

For each `InitTarget`, it will spawn a multicluster-runtime manager that will process all
clusters that use the `WorkspaceType` referenced in the target. Whenever a new cluster is created, it
will be reconciled.

Reconciliation involves getting the most recent version of the `InitTarget` and then fetching each
of its init sources. Each source can provide any number of Kubernetes objects that are first sorted
to ensure CRDs come first, then namespaces etc., and then applied to the newly created cluster.

If a resource cannot be found, it is assumed that this is a transient error (for example when one
init source creates an object using a resource provided by yet another mechanism (maybe another
initializer, maybe a default APIBinding)). In these cases, reconciliation is retried after a few
seconds.

If all objects from all sources were applied cleanly, the initializer is removed from the
`LogicalCluster`, which ends the agent's involvement and makes it "disappear" from the agent.

[kcp]: https://kcp.io
