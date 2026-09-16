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
| `SubDomain` | no | The subdomain used to reach this instance when a domain name is attached. Not yet acted on — see limitations. |
| `KubernetesRuntimeInstanceID` | no | The runtime to deploy to. Falls back to the control plane's default runtime when unset. |
| `DjangoDefinitionID` | yes | The definition this instance deploys. |

## Status

What works today, updated as the module progresses.

| Piece | State |
|---|---|
| SDK config and API objects | done |
| Generated API server, client, controller scaffolding | done |
| Kubernetes manifests for the Django app | done |
| Definition reconciler | done |
| Instance reconciler | done |
| Config abstractions (`pkg/config`) | done — see `samples/` |
| tptctl plugin | builds, installs, and serves its subcommands |
| Verified against a live control plane | yes — see below |
| Demo application and deploy example | `examples/` |

## What has been verified

The module was installed into a Threeport control plane on kind and exercised
end to end with a minimal Django image:

- creating a `DjangoDefinition` produces a `KubernetesWorkloadDefinition`
- creating a `DjangoInstance` before its definition finished reconciling
  requeues and succeeds on the retry, rather than failing
- the workload deploys seven resources: the database secret, its volume claim,
  a PostgreSQL deployment and service, the migration job, and the application
  deployment and service
- the migration job completes on its first attempt
- the application starts, stays ready, and reaches its database: the demo
  image's health endpoint answers `{"status": "ok", "database": "reachable"}`
- deleting the instance removes the workload, and the API refuses to delete a
  definition that still has instances
- deleting the definition removes the workload definition

Three things came out of those runs and are now fixed.

**The migration job could not import the project.** It failed with
`ModuleNotFoundError` while the application started fine. `django-admin` is an
installed console script, so Python puts its own directory on `sys.path` and
not the project's; a server such as gunicorn adds the working directory itself,
which is why only the job broke. The job now sets `PYTHONPATH=.`, which assumes
the image keeps its project at the working directory — the usual layout.

**The migration job raced the database.** Started alongside PostgreSQL, its
first attempt failed with `connection refused` and only the retry succeeded.
That left migrations one slow database start from failing outright, since
`backoffLimit` is 1. The job now waits on an init container running `pg_isready`
rather than leaning on the retry, which also keeps a genuine migration failure
from being retried against a healthy database.

**Namespaces were declared and ignored.** The manifests set
`namespace: default` on every object, and Threeport assigned its own namespace
per workload instance regardless. The declarations were removed rather than
left to imply a control the module does not have.

### A note on verifying fixes here

The first attempt at the `PYTHONPATH` fix passed its unit test and was not
running: the images are rebuilt under the same `v0.0.1-dev` tag, so the kubelet
kept serving the cached one and the controller went on emitting the old
manifest. Changes to manifest generation are only proven by redeploying and
reading the resulting object, not by the test suite. Set the controller's
`imagePullPolicy` to `Always`, or install with `--debug`, before concluding a
change took effect.

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

**The database password is regenerated on every render.** `djangoYaml` mints a
new credential each time it runs, so re-rendering an existing definition would
produce a manifest the running database does not accept. The definition
reconciler avoids this by adopting an existing workload definition rather than
rewriting it, but an update path that re-renders will have to read the current
secret instead.

**`SubDomain` is stored but not acted on.** Reaching an instance by subdomain
needs a gateway and a domain name attached to it, which is a second set of
Threeport objects this module does not create yet. The field is modelled so the
API does not have to change when it is implemented.

**`ALLOWED_HOSTS` is derived,** not configured, from the domain name and
subdomain attached to an instance.

## Threeport version

This module tracks the tip of the Threeport `0.7` branch rather than a release.
The SDK that generates it emits calls to `ProcessCoreTaggedFields*`, which do
not exist in `v0.6.1`, the most recent published release. Pin this to a release
once the `0.7` line has one.

## Trying it out

`examples/django-demo` is a minimal Django application built for this: it
serves a health endpoint that queries the database, so a successful response
proves the application started, found its settings, and reached PostgreSQL.

It is deliberately small — two installed apps, no static files, no admin — so a
failure points at the module rather than at the application.

### 1. Build and publish the demo image

The dev flow pulls from the local registry that `mage dev:localRegistryUp`
starts on port 5001, not from `ImageNamespace`.

```bash
cd examples/django-demo
docker build -t localhost:5001/django-demo:v0.1.0 .
docker push localhost:5001/django-demo:v0.1.0
```

### 2. Build and install the module

```bash
mage build:allImagesDev     # api, database migrator, controller
mage install:plugin         # puts the tptctl plugin in ~/.threeport/plugins
tptctl django install -r localhost:5001
```

`-r localhost:5001` matters: the install defaults to `ImageNamespace`, and the
dev images are in the local registry.

### 3. Deploy an application

```bash
tptctl django create django -c samples/django.yaml
```

`samples/django.yaml` creates a definition and an instance that share a name -
a defined instance. `samples/django-definition.yaml` and
`samples/django-instance.yaml` create either half on its own, which is what to
use for several instances of one definition.

A config file is read strictly: a field the values objects do not carry is an
error rather than something quietly ignored. `Image` is required, `Name` and
`Environment` have to be usable as Kubernetes label values, and `Replicas`
cannot be negative - all of them reported before anything is sent to the API.

A replace keeps an instance on the runtime it is already on. Omitting
`KubernetesRuntimeInstance` means "the default" on a create and "leave it where
it is" on a replace, so editing an unrelated field in a config that does not
name a runtime cannot move a running workload to another cluster. Naming a
different one is reported rather than performed.

`examples/deploy` does the same thing through the client library. It predates
the config abstractions and is kept because it is a compact example of driving
the module from Go.

Watch it arrive. Threeport assigns a namespace per workload instance, so find
it rather than assuming `default`:

```bash
NS=$(kubectl get ns -o name | grep myapp- | sed 's|namespace/||')
kubectl -n $NS get pods
```

Expect three: PostgreSQL running, the migration job `Completed`, and the
application running. Then reach the application:

```bash
kubectl -n $NS port-forward svc/myapp 8080:80
curl localhost:8080
# {"status": "ok", "database": "reachable"}
```

### 4. Clean up

```bash
tptctl django delete django -c samples/django.yaml
```

Deletion is asynchronous: the API marks the instance and the reconciler tears
the workload down before the row goes away. `DjangoInstanceConfig.Delete` waits
for that, so removing a defined instance no longer fails on the definition
still having instances attached.

`examples/deploy` has no such wait, so `go run ./examples/deploy -name myapp
-delete` can still report that instances exist. Run it again after a few
seconds.

## Development

```bash
threeport-sdk gen --config sdk-config.yaml   # regenerate after changing pkg/api
go build ./...
mage build:plugin                            # build the tptctl plugin
```
