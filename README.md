# ToBAC is superseded by dedicated team namespaces

ToBAC provides _Team-Based Access Control_ to Kubernetes clusters. It is implemented as a
[Validating Admission Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/).
Kubernetes will send admission requests to this daemon and expects ToBAC to decide whether or not
a user or service account will gain access to certain cluster resources.

## Mode of operation

ToBAC's source of truth is a combination between command-line parameters and this
[list of teams maintained in Sharepoint](https://navno.sharepoint.com/sites/Bestillinger/Lists/Nytt%20Team/AllItems.aspx).
Cluster administrators are exempt from permission checking and must specified upon daemon startup.

1. The Kubernetes API server receives a write request intersecting with the ruleset specified below
2. The API server uses RBAC rules to decide whether or not the request should succeed
3. The API server then sends a HTTP query to ToBAC asking for permission
4. ToBAC decides whether or not access will be granted based on the resource's `team` label.
5. If the write operation in question is a DELETE operation, ToBAC must query the Kubernetes API server to get the original resource.
6. In case of server failure, access is only granted if `failurePolicy` is set to `Ignore`. This should be turned off

## Developer access ruleset

This ruleset is governed by the ClusterRole and ClusterRoleBinding `nais:developer`.
The actual Kubernetes resources can be found in the [nais-yaml](https://github.com/navikt/nais-yaml) repository.

The label `team-only` means that the object will only be created if it has a `team: <team>` metadata label.
Similarily, `same-team` implies that the action will only be possible if the user belongs to the same team as specified in the metadata label.

Resources that do not have a `team` label cannot be created, but they can be modified by anyone,
provided that a `team` label is specified in the updated resource.

Creation:

- Applications (`team-only`, `same-team`)
- ConfigMaps (`team-only`, `same-team`)
- RedisFailovers (`team-only`, `same-team`)
- Pods/exec (`same-team`)

Updates:

- Applications (`same-team`)
- ConfigMaps (`same-team`)
- RedisFailovers (`same-team`)

Deletion:

- Applications (`same-team`)
- ConfigMaps (`same-team`)
- RedisFailovers (`same-team`)
- Pods (`same-team`)
