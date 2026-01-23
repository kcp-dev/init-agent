# Frequently Asked Questions

## Can I add/update objects to already initialized clusters?

No. Once the initialization is completed (and the initializer removed from the
`LogicalCluster`), the init-agent has no way to access the newly created cluster
anymore, so it also cannot reconcile after that point.

Reconciling behaviour is planned for future releases, but the exact approach is
not yet decided.

## Can `InitTargets` generate random data, like passwords?

Yes! `InitTemplates` can make use of the entire [sprig][sprig] library, which
includes functions to generate passwords, UUIDs etc.

However, the init-agent has no memory, so if one initialization round fails
and then agent re-tries, it will re-render all templates and so generate new,
different random data.

[sprig]: https://masterminds.github.io/sprig/
