<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Chart and Helm templates

Patterns where this repo's Helm chart under `charts/lfx-v2-project-service/`
renders invalid YAML, ships unsafe defaults, or references a values path that
does not exist. The chart's `templates/ruleset.yaml`, `templates/httproute.yaml`,
`templates/heimdall-middleware.yaml`, and `values.yaml` have been the single most
review-flagged surface in this repo's history (CodeRabbit, Copilot, and human
maintainers). Helm-template control-line indentation in particular recurs across
many PRs because `{{- if }}` lines that sit at list-item indentation collapse to
spaces-only lines and break `helm template | kubectl apply`.

**Read when:** any file under `charts/lfx-v2-project-service/**` changed —
especially `templates/ruleset.yaml`, `templates/httproute.yaml`,
`templates/heimdall-middleware.yaml`, `templates/deployment.yaml`,
`templates/nats-kv-buckets.yaml`, `templates/nats-object-stores.yaml`, or
`values.yaml`.

---

## `chart-and-helm/control-line-indentation` — Critical

**Pattern:** a Helm control statement (`{{- if }}` / `{{- else if }}` / `{{- end }}`) is indented to the same column as the YAML list items it wraps (e.g. under `execute:`, `containers:`, `filters:`). The braces trim only the newline, so when the branch is false the leading spaces remain and produce a spaces-only line under a mapping key without a `-`, breaking the rendered document.

**Detect:** in any chart template, find `{{- if`/`{{- else`/`{{- end` lines that are indented to the same depth as adjacent `- ` list items under a mapping key. Render with `helm template` (or check the YAMLlint signature `expected the node content, but found '-'` / `could not find expected ':'`). Pay special attention to every `execute:` block in `ruleset.yaml`.

**Empirical citation:** PR #10 `charts/lfx-v2-project-service/templates/ruleset.yaml:22` (CodeRabbit) — "All three Helm control lines (`if / else if / end`) are indented to the same level as list items. Because they trim only the newline _inside_ the braces, the eight leading spaces remain, producing blank 'spaces-only' lines beneath `execute:` ... That yields `helm template | kubectl apply` failures". Also PR #7 `templates/httproute.yaml:34` (CodeRabbit, "`filters:` block renders invalid YAML when Heimdall is disabled") and PR #10 `templates/deployment.yaml:26` (CodeRabbit, "Indentation breaks Helm-rendered YAML"). All resolved by code change.

**Failure message:** Helm control line is indented as a list item; when the branch is false it emits a spaces-only line that breaks the rendered YAML.

**Fix:** out-dent the control statements so they do not participate in YAML structure, or move the wrapped mapping key (e.g. `filters:`) inside the conditional so the key is only emitted when its list is populated. Verify with `helm template ./charts/lfx-v2-project-service`.

---

## `chart-and-helm/values-path-mismatch` — Critical

**Pattern:** a template references a `.Values` path that does not match the structure declared in `values.yaml` (e.g. `.Values.app.heimdall.add_middleware` when the key lives at `.Values.heimdall.add_middleware`). The condition silently never fires, so the resource is never deployed (or always falls through to the wrong branch).

**Detect:** for each `.Values.<path>` reference in a changed template, confirm the same nested path exists in `values.yaml`. Watch conditionals (`{{ if .Values... }}`) whose whole resource is gated on the path.

**Empirical citation:** PR #23 `charts/lfx-v2-project-service/templates/heimdall-middleware.yaml:3` (Copilot) — "The template references `.Values.app.heimdall.add_middleware` but the configuration is defined at `.Values.heimdall.add_middleware` in values.yaml. This will cause the middleware to never be deployed." Resolved by code change.

**Failure message:** Template `.Values` path does not exist in values.yaml — the conditional silently never fires.

**Fix:** correct the `.Values` path to match `values.yaml`. If the value is meant to be nested under `app`, add it there; otherwise reference the existing key.

---

## `chart-and-helm/double-url-scheme` — Important

**Pattern:** a template prepends `http://` to a `.Values` URL that already includes the scheme, producing a malformed address like `http://http://lfx-platform-heimdall.lfx.svc.cluster.local:4456`.

**Detect:** grep changed templates for `http://{{ .Values` (or `https://{{ .Values`) where the referenced value in `values.yaml` already starts with a scheme.

**Empirical citation:** PR #23 `charts/lfx-v2-project-service/templates/heimdall-middleware.yaml:17` (Copilot) — "The address configuration incorrectly adds `http://` prefix to `.Values.heimdall.url`, but the URL in values.yaml already includes the `http://` scheme. This will result in malformed URLs like `http://http://...`. Remove the `http://` prefix and use `{{ .Values.heimdall.url }}` directly." Resolved by code change.

**Failure message:** Hard-coded `http://` prefix in front of a values URL that already carries a scheme — produces a double-scheme address.

**Fix:** reference the values key directly (`{{ .Values.heimdall.url }}`). Keep the scheme in `values.yaml`, not in the template.

---

## `chart-and-helm/unsafe-default-enabled` — Important

**Pattern:** `values.yaml` ships a default that is unsafe for fresh installs — `useLocalImage: true`, `authelia.enabled: true`, `appVersion: "latest"`, or `imagePullPolicy: Never` on an image that may not exist locally. Production / CI / partner clusters that do not match the assumed environment then break (pull failures, `401 unknown authenticator`).

**Detect:** in `values.yaml` and `Chart.yaml`, flag defaults that assume a specific local/dev environment: `useLocalImage: true`, an authenticator `enabled: true` that only some clusters deploy, `appVersion`/image tag set to `latest`, or `imagePullPolicy: Never`.

**Empirical citation:** PR #10 `charts/lfx-v2-project-service/values.yaml:119` (CodeRabbit) — "`authelia.enabled` is committed as `true` ... In clusters that do not deploy Authelia ... Heimdall will return _401 unknown authenticator_. Safer default is to ship both options disabled". Also PR #10 `values.yaml:13` (CodeRabbit, `useLocalImage` should default `false`) and PR #10 `Chart.yaml:9` (Copilot, "Using 'latest' as appVersion is not recommended for production").

**Failure message:** Chart ships an environment-specific default enabled by default — breaks clusters that don't match the assumption.

**Fix:** default environment-specific toggles to `false` (or pin a concrete version) and let the platform/umbrella chart or an overlay values file opt in explicitly.

---

## `chart-and-helm/internal-routes-exposed` — Nit

**Pattern:** `/livez` and `/readyz` (or other internal-only paths) are routed through the HTTPRoute/IngressRoute. Kubernetes reaches these directly via the service ClusterIP; exposing them through Traefik is unnecessary surface.

**Detect:** in `templates/httproute.yaml` (or any ingress template), look for `/livez` or `/readyz` path matches.

**Empirical citation:** PR #1 `charts/lfx-v2-project-service/templates/ingressroute.yaml:19` (bramwelt) — "`/livez` and `/readyz` don't need to be routed to as they're used internally by Kubernetes." Also PR #7 `templates/httproute.yaml:46` (bramwelt) — "These routes don't need to be exposed through Traefik as Kubernetes already has access ... through the service Cluster IP."

**Failure message:** Health endpoints (`/livez` / `/readyz`) routed through the ingress — unnecessary external exposure.

**Fix:** remove the health-check paths from the ingress route. Note: a contributor reported a 404 when removing them naively (PR #1) — verify the route still serves the real API paths after the change.
