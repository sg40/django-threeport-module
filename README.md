# Django Threeport Module

A [Threeport](https://threeport.io) module that manages Django application
deployments.

Like every Threeport module, this is a Go project. Django is the workload the
module deploys, not the language it is written in: the controller reconciles
`DjangoDefinition` and `DjangoInstance` objects by generating Kubernetes
manifests and handing them to the Threeport API as a Kubernetes workload.

## Objects

**`DjangoDefinition`** describes how a Django application is built and
configured. It is the reusable part: one definition can back many instances.

| Field | Required | Notes |
|---|---|---|
| `Image` | yes | The application's container image. Unlike WordPress, Django has no canonical public image — every project builds its own — so the module cannot deploy anything without it. |
| `SettingsModule` | no | Passed to the app as `DJANGO_SETTINGS_MODULE`, e.g. `myapp.settings.production`. |
| `Environment` | no | Drives defaults such as replica count. Defaults to `dev`. |
| `Replicas` | no | Overrides the replica count derived from `Environment`. |
| `RunMigrations` | no | Runs `django-admin migrate` before the app is made available. Defaults to `true`, because Django requires it on any schema change. |

**`DjangoInstance`** is a running deployment of a definition.

| Field | Required | Notes |
|---|---|---|
| `SubDomain` | no | The subdomain used to reach this instance when a domain name is attached. |
| `DjangoDefinitionID` | yes | The definition this instance deploys. |

## Status

What works today, updated as the module progresses.

| Piece | State |
|---|---|
| SDK config and API objects | done |
| Generated API server, client, controller scaffolding | done |
| Kubernetes manifests for the Django app | not started |
| Definition reconciler | scaffolded, no business logic |
| Instance reconciler | scaffolded, no business logic |
| Config abstractions (`pkg/config`) | generated, not customised |
| tptctl plugin | generated, not built |

## Known limitations

**No managed database.** The database is always a containerized Postgres
deployed alongside the application. The WordPress module offers a
`ManagedDatabase` flag that delegates to an `AwsRelationalDatabaseDefinition`,
but that object does not exist in the Threeport `0.7` line — the only AWS
objects there are `AwsProvider`, `AwsEksKubernetesRuntimeDefinition` and
`AwsEksKubernetesRuntimeInstance`. The field was left out rather than accepted
and ignored. It can be added once Threeport offers a managed database object
again.

**Secrets are not modelled.** `SECRET_KEY` does not belong in a database
column. Threeport has a `Secret` object that is the right home for it, but
wiring it in adds a dependency between modules, so it is deferred.

**`ALLOWED_HOSTS` is derived,** not configured, from the domain name and
subdomain attached to an instance.

## Threeport version

This module tracks the tip of the Threeport `0.7` branch rather than a release.
The SDK that generates it emits calls to `ProcessCoreTaggedFields*`, which do
not exist in `v0.6.1`, the most recent published release. Pin this to a release
once the `0.7` line has one.

## Development

```bash
threeport-sdk gen --config sdk-config.yaml   # regenerate after changing pkg/api
go build ./...
mage build:plugin                            # build the tptctl plugin
```
