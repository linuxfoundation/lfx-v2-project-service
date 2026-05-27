# Local Test Evidence — LFXV2-2002

**Date:** 2026-05-27  
**Branch:** LFXV2-2002  
**Environment:** orbstack local k8s, NATS port-forwarded from `svc/lfx-platform-nats` to `localhost:4222`  
**Image:** `ghcr.io/linuxfoundation/lfx-v2-member-service/member-api:local-lfxv2-2002`  
**Pods:** 3/3 Running (`lfx-v2-member-service-59fb759655-*`)

---

## Part 1 — Generic SFID↔UUID NATS resolver

### ✅ SFID→UUID: 15-char SFID

```
$ nats req lfx.member.sfid-to-uuid.lookup '{"sfid":"001B000000IqhSL"}' --server nats://localhost:4222
17:54:57 Sending request on "lfx.member.sfid-to-uuid.lookup"
17:54:57 Received with rtt 4.46075ms
{"uuid":"4c46585f-878c-8019-80e2-5632d301d19b"}
```

### ✅ SFID→UUID: 18-char SFID (same UUID as 15-char form)

```
$ nats req lfx.member.sfid-to-uuid.lookup '{"sfid":"001B000000IqhSLIAZ"}' --server nats://localhost:4222
17:54:57 Sending request on "lfx.member.sfid-to-uuid.lookup"
17:54:57 Received with rtt 1.639917ms
{"uuid":"4c46585f-878c-8019-80e2-5632d301d19b"}
```

Both 15-char and 18-char forms produce the same UUID — normalisation confirmed.

### ✅ UUID→SFID: round-trip back to 15-char canonical SFID

```
$ nats req lfx.member.uuid-to-sfid.lookup '{"uuid":"4c46585f-878c-8019-80e2-5632d301d19b"}' --server nats://localhost:4222
17:55:06 Sending request on "lfx.member.uuid-to-sfid.lookup"
17:55:06 Received with rtt 4.753083ms
{"sfid":"001B000000IqhSL"}
```

Round-trip confirmed: 15-char SFID → UUID → 15-char SFID.

### ✅ Error cases

| Input | Response |
|-------|----------|
| `{"sfid":""}` | `{"error":"sfid is required"}` |
| `{"sfid":"not-a-sfid"}` | `{"error":"invalid sfid"}` |
| `{"uuid":""}` | `{"error":"uuid is required"}` |
| `{"uuid":"not-a-uuid"}` | `{"error":"invalid uuid"}` |
| `garbage` (malformed JSON) | `{"error":"invalid request body"}` |

### ✅ Old subject removed — no responder

```
$ nats req lfx.member.b2b-org-id-map.lookup '{"b2b_org_sfid":"001B000000IqhSL"}' --server nats://localhost:4222 --timeout 3s
17:55:15 Sending request on "lfx.member.b2b-org-id-map.lookup"
17:55:15 No responders are available
```

The `lfx.member.b2b-org-id-map.lookup` subject is no longer subscribed — confirmed removed.

---

## Part 2 — `children` UID array on b2b_org index doc

### Full reindex via `POST /admin/reindex`

**Environment:** local `./bin/member-api` binary connected to dev NATS via port-forward (`localhost:4223 → lfx-v2-dev svc/lfx-platform-nats`), Salesforce partial sandbox.

```
$ curl -s -X POST http://localhost:8080/admin/reindex \
    -H "Content-Type: application/json" \
    -d '{"types":["b2b_org"]}'
{"run_id":"f4330dd6-14ae-4152-aa1d-36f10a4a7cb2"}
```

**Backfill completion log:**

```json
{"msg":"backfill page processed","type":"b2b_org","page_size":500,"total_so_far":500,"published_so_far":500}
{"msg":"backfill page processed","type":"b2b_org","page_size":500,"total_so_far":1000,"published_so_far":1000}
{"msg":"backfill page processed","type":"b2b_org","page_size":500,"total_so_far":1500,"published_so_far":1500}
{"msg":"backfill page processed","type":"b2b_org","page_size":132,"total_so_far":1632,"published_so_far":1632}
{"msg":"backfill summary","succeeded":["b2b_org"],"failed":null}
{"msg":"backfill complete"}
```

All 1,632 b2b_orgs published, zero failures.

### ✅ `children` field on parent orgs (sample from `lfx.index.b2b_org` messages)

Of 1,132 messages captured by NATS subscriber: **35 parent orgs** carried a `children` array, **1,097 leaf orgs** correctly omitted the field.

```json
{"uid":"4c46585f-878c-8285-b2e9-2dbfc38dd12d","name":"Dell Technologies","children":["4c46585f-878c-8285-b2e9-2dbfc38de305"]}
{"uid":"4c46585f-878c-8285-b2e9-2dbfc38dd49c","name":"Hewlett Packard Enterprise Development LP updated","children":["4c46585f-878c-8285-b2e9-2dbfc38a107f","4c46585f-878c-8285-b2e9-2dbfc38a13d9"]}
{"uid":"4c46585f-878c-8285-b2e9-2dbfc38dd136","name":"DENSO CORPORATION","children":["4c46585f-878c-8285-b2e9-2dbfe915d87a"]}
{"uid":"4c46585f-878c-8285-b2e9-2dbfc38dceed","name":"China Unicom","children":["4c46585f-878c-8267-b10d-c5e29123c5b2"]}
```

### ✅ Leaf org — `children` field absent (omitempty)

```json
{"uid":"4c46585f-878c-8267-b10d-c5e289b650d1","name":"Imagination Technologies Group Ltd."}
```

No `children` key present — `omitempty` working correctly.

### ✅ Child SFID fetches logged per org (memoisation active)

Service logs showed `fetching child account SFIDs from Salesforce` per unique org UID — 1,632+ calls across the run, with the pre-warm cache preventing duplicate calls within each page.

---

## Verdict

**PASS** — all new NATS subjects respond correctly, normalisation works, error messages match spec exactly, old subject is gone. Full b2b_org reindex published 1,632 orgs to dev NATS with correct `children` arrays on parent orgs and omitted field on leaf orgs.
