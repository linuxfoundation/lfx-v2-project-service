<!-- Copyright The Linux Foundation and each contributor to LFX. -->
<!-- SPDX-License-Identifier: MIT -->

# Service Helm Chart

This document owns member-service-specific chart behavior. Shared chart
conventions live in `lfx-v2-helm`; deployed values, the Salesforce
ExternalSecret, IRSA role, region, and chart pins live in `lfx-v2-argocd`.

- **Chart path**: `charts/lfx-v2-member-service/`
- **Service type**: Salesforce-backed membership API with local NATS KV
  caches.
- **HTTPRoute paths**: `/projects/{uid}/tiers(/.*)?`,
  `/projects/{uid}/memberships(/.*)?`, `/b2b_orgs`,
  `/b2b_orgs/{uid}/memberships(/.*)?`, `/_memberships/`.
  `/debug/vars`, `/livez`, and `/readyz` are mounted by the binary but are
  not exposed by this HTTPRoute.
- **NATS KV buckets**: `membership-cache`, `member-service-cache`.
- **B2B detour auth**: `/b2b_orgs` rules currently check `auditor` on the
  static project UID from `openfga.lfProjectUID`; this is an interim
  workaround documented in `ARCHITECTURE.md`.
- **Salesforce secret**: the chart references the pre-existing Kubernetes
  Secret named by `values.yaml` at `salesforce.secrets.name`
  (`lfx-v2-member-service-salesforce` by default). The chart does not create
  the ExternalSecret.
- **Service account**: `serviceaccount.yaml` renders optional annotations from
  `serviceAccount.annotations`; IRSA annotation values are supplied by
  deployment configuration, not hardcoded in this chart.
- **Env wiring**: standard service env plus Salesforce credential refs from
  `salesforce.secrets.keys`.
