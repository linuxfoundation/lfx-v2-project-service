<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Helm chart and deployment

Patterns in this service's Helm chart and Heimdall ruleset that reviewers (Copilot +
maintainers) repeatedly flag: OpenFGA object IDs rendered from an empty value, env vars
rendered without `| quote`, namespace hardcoding, and HTTPRoute / authorizer references
that don't match the deployed platform. The empty-OpenFGA-object case is Critical (it
silently denies or mis-authorizes); the rest are Important.

**Read when:** any file under `charts/lfx-v2-member-service/**`. Also read
`docs/service-helm-chart.md`. Cross-checked in Steps 3-4 of the learnings-review
playbook.

---

## `chart-and-deploy/empty-openfga-object-id` — Critical

**Pattern:** the Heimdall `ruleset.yaml` builds an OpenFGA object from a chart value
(`lfProjectUID`, `globalOrgAdminTeamUID`) that defaults to `""` while `openfga.enabled`
defaults to `true`. An unset value renders `object: "project:"` / `team:` (empty ID), so
the FGA check fails or silently mis-authorizes. The repo's established remedy is the
`"_null"` sentinel default so the check fails closed and noisily rather than rendering an
empty object.

**Detect:** in `charts/lfx-v2-member-service/templates/ruleset.yaml` and `values.yaml`,
find every OpenFGA `object:` built from a chart value; verify the value's default is the
`"_null"` sentinel (not `""`) when `openfga.enabled` is true, or that a Helm-time
fail-fast guards it.

**Empirical citation:** PR #11 `charts/lfx-v2-member-service/templates/ruleset.yaml:83` — Copilot — "`openfga.lfProjectUID` defaults to an empty string in values.yaml while `openfga.enabled` defaults to true. If this value isn't set at deploy time, this rule will render `object: \"project:\"` and OpenFGA checks will likely fail". Acted on PR #26: emsearcy — "an empty value could cause problems with the FGA check. Let's update this to `\"_null\"`" → prabodhcs — "`lfProjectUID` default changed from `\"\"` to `\"_null\"`." Reinforced PR #38 `values.yaml:44` (dealako: `_null` now gates write endpoints — accidental default deploy silently denies all org-admin mutations).

**Failure message:** OpenFGA object built from an empty-default chart value — renders `project:`/`team:` with no ID; FGA check fails or mis-authorizes.

**Fix:** default the value to the `"_null"` sentinel (never `""`) so the FGA check fails
closed, and/or add a Helm-time `required`/fail-fast under the `openfga.enabled` branch.

---

## `chart-and-deploy/env-value-missing-quote` — Important

**Pattern:** a string env var in `deployment.yaml` is rendered as `value: {{ .Values... }}`
without `| quote`. If the value is supplied as a non-string (e.g. `--set
app.x=123`), the rendered manifest has a non-string `env.value`, which Kubernetes rejects.

**Detect:** in `charts/lfx-v2-member-service/templates/deployment.yaml` and
`deployment-consumer.yaml`, find `value:` lines templated from a chart value that lack
`| quote`; flag any string env var without it.

**Empirical citation:** PR #38 `charts/lfx-v2-member-service/templates/deployment.yaml:42` — Copilot — "`GLOBAL_ORG_ADMIN_TEAM_UID` is rendered without `| quote`. If this value is set via `--set app.globalOrgAdminTeamUID=123` ... the rendered manifest will contain a non-string `env.value`, which Kubernetes rejects." Acted on: prabodhcs — "added `| quote` to force string rendering. Consistent with all other string env vars in the file."

**Failure message:** String env var rendered without `| quote` — a non-string override produces a manifest Kubernetes rejects.

**Fix:** add `| quote` to the templated env value, consistent with the other string env
vars in the file.

---

## `chart-and-deploy/hardcoded-namespace` — Important

**Pattern:** a chart template hard-codes `metadata.namespace: lfx` (or another fixed
name) instead of using the release/values namespace, preventing install into a different
namespace and diverging from sibling templates.

**Detect:** in `charts/lfx-v2-member-service/templates/**`, grep for literal
`namespace: lfx` (or other hardcoded `metadata.namespace`); compare against templates that
use `.Release.Namespace` / `.Values.lfx.namespace`.

**Empirical citation:** PR #33 `charts/lfx-v2-member-service/templates/nats-kv-buckets.yaml:15` — Copilot — "`metadata.namespace` is hard-coded to `lfx`, which prevents installing this chart into a different namespace (and diverges from other templates that use `.Release.Namespace` or `.Values.lfx.namespace`)." (Flagged twice in the same file at lines 15 and 38.)

**Failure message:** Chart template hard-codes `metadata.namespace` — chart cannot install into a non-`lfx` namespace and diverges from sibling templates.

**Fix:** template the namespace from `.Values.lfx.namespace` / `.Release.Namespace`
consistent with the other templates in the chart.

---

## `chart-and-deploy/undefined-or-shared-authorizer` — Important

**Pattern:** a `ruleset.yaml` rule references a Heimdall authorizer that isn't defined in
this chart. This is acceptable ONLY when the authorizer is a platform-level component
deployed by the shared `lfx-platform` chart (e.g. `json_content_type`, `allow_all`) and
already used verbatim by a sibling service. An authorizer that is neither a Heimdall
built-in, a platform component, nor defined here will fail at runtime.

**Detect:** in `charts/lfx-v2-member-service/templates/ruleset.yaml`, list each
`authorizer:` referenced; confirm each is either defined in this chart, a documented
platform/shared authorizer, or a Heimdall built-in. Flag an undocumented one.

**Empirical citation:** PR #38 `charts/lfx-v2-member-service/templates/ruleset.yaml:207` — Copilot — "This RuleSet now references an authorizer named `json_content_type`, but this chart/repo does not appear to define or configure that authorizer anywhere. If `json_content_type` is not a Heimdall built-in or pre-installed platform component, this will fail at runtime". Resolved as a known platform authorizer: prabodhcs — "`json_content_type` is a platform-level Heimdall authorizer deployed as part of the shared lfx-platform Helm chart ... already used verbatim in `lfx-v2-committee-service`". (This makes the pattern Important-with-context, not a hard fail.)

**Failure message:** Heimdall ruleset references an authorizer not defined here — confirm it is a documented platform/shared authorizer, else it fails at runtime.

**Fix:** confirm the authorizer is a Heimdall built-in or a shared `lfx-platform`
authorizer already used by a sibling service; if so, leave a comment noting that. If it is
truly local, define it in this chart.

---

## `chart-and-deploy/unanchored-or-broad-httproute` — Important

**Pattern:** an HTTPRoute `RegularExpression` path is unanchored (risking unintended
partial matches and implementation-specific precedence vs `PathPrefix`), or the
unauthenticated OpenAPI route matches the entire `/_memberships/` prefix instead of the
small fixed set of spec files, risking accidental public exposure of future endpoints
added under that prefix.

**Detect:** in `charts/lfx-v2-member-service/templates/httproute.yaml`, check
`RegularExpression` values are anchored and that the `allow_all`/unauthenticated route is
narrowed to the actual OpenAPI spec files (not a broad prefix).

**Empirical citation:** PR #16 `charts/lfx-v2-member-service/templates/httproute.yaml:23` — Copilot — "The `RegularExpression` path values are not anchored. Depending on Gateway implementation regex semantics, this can lead to unintended partial matches". Same PR `httproute.yaml:63` — "The unauthenticated OpenAPI route matches the entire `/_memberships/` prefix ... to avoid accidentally exposing future endpoints added under `/_memberships/`, consider narrowing the gateway match".

**Failure message:** HTTPRoute regex unanchored or unauthenticated route too broad — partial-match precedence surprises or accidental public exposure.

**Fix:** anchor the `RegularExpression` paths and narrow the unauthenticated route to the
exact OpenAPI spec files. Keep public exposure consistent with the documented public paths
in `docs/service-helm-chart.md`.
