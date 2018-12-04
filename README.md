# tobac

Kubernetes validating admission webhook.

## Developer access

Governed by the role `nais:team`. Developers should have access to the following cluster operations.

The label `team-only` means that the object will only be created if it has a `team: <team>` metadata label.
Similarily, `same-team` implies that the action will only be possible if the user belongs to the same team as specified in the metadata label.

Creation:

- Applications (team-only, same-team)
- ConfigMaps (team-only, same-team)
- RedisFailovers (team-only, same-team)
- Pods/exec (same-team)

Updates:

- Applications (same-team)
- ConfigMaps (same-team)
- RedisFailovers (same-team)

Deletion:

- Applications (same-team)
- ConfigMaps (same-team)
- RedisFailovers (same-team)
- Pods (same-team)

Read-only access; get, list, and watch. These are granted by the `view` role.

- Pods
- Pods/log
- Deployments
- Services
- Ingresses
- ConfigMaps
- HorizontalPodAutoscalers
