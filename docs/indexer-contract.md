# Indexer Contract — Member Service

This document is the authoritative reference for all data the member service sends to the indexer service, which makes resources searchable via the [query service](https://github.com/linuxfoundation/lfx-v2-query-service).

**Update this document in the same PR as any change to indexer message construction.**

---

## Resource Types

- [B2B Org](#b2b-org)
- [Project Membership](#project-membership)
- [Key Contact](#key-contact)

---

## B2B Org

**Object type:** `b2b_org`

**NATS subject:** `lfx.index.b2b_org`

**Source struct:** `internal/domain/model/b2b_org.go` — `B2BOrg`

**Indexed on:** create, update, delete of a B2B org.

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | B2B org unique identifier |
| `name` | string | Organization display name |
| `description` | string (optional) | Free-text description |
| `phone` | string (optional) | Contact phone number |
| `website` | string (optional) | Website URL |
| `primary_domain` | string (optional) | Canonical primary domain |
| `domain_aliases` | []string (optional) | Additional normalized domains |
| `logo_url` | string (optional) | Logo image URL |
| `industry` | string (optional) | Industry classification |
| `sector` | string (optional) | Sector classification |
| `crunch_base_url` | string (optional) | CrunchBase profile URL |
| `number_of_employees` | int64 (optional) | Employee count |
| `status` | string (optional) | LF membership status |
| `is_member` | bool | Whether the org is an active LF member |
| `parent_uid` | string (optional) | UID of the parent org |
| `parent_detail` | object (optional) | Denormalized parent info: `uid`, `name`, `logo_url` |
| `is_parent` | bool (optional) | `true` when this org has at least one direct member-eligible child. Omitted when false. To retrieve children, query the index for `parent_uid = <this uid>`. |
| `created_at` | timestamp | Creation time (RFC3339) |
| `updated_at` | timestamp | Last update time (RFC3339) |

### Tags

| Tag Format | Example | Purpose |
|---|---|---|
| `{uid}` | `cbef1ed5-...` | Direct lookup by UID |
| `b2b_org_uid:{uid}` | `b2b_org_uid:cbef1ed5-...` | Find orgs by UID |
| `parent_b2b_org_uid:{uid}` | `parent_b2b_org_uid:abc-...` | Find all children of a parent org |
| `is_member:{true\|false}` | `is_member:true` | Filter by LF member status |

> `parent_b2b_org_uid` tag is only emitted when `parent_uid` is non-empty.

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `b2b_org:{uid}` |
| `access_check_relation` | `auditor` |
| `history_check_object` | `b2b_org:{uid}` |
| `history_check_relation` | `auditor` |

### Search Behavior

| Field | Value |
|---|---|
| `fulltext` | `name`, `primary_domain`, `description`, `industry`, `sector` |
| `name_and_aliases` | `name`, `primary_domain`, all `domain_aliases` |
| `sort_name` | `name` (lowercased) |
| `public` | `false` |

### Parent References

| Ref | Condition |
|---|---|
| `b2b_org:{parent_uid}` | Only when `parent_uid` is set |

---

## Project Membership

**Object type:** `project_membership`

**NATS subject:** `lfx.index.project_membership`

**Source struct:** `internal/domain/model/membership.go` — `ProjectMembership`

**Indexed on:** create via `/admin/reindex` (memberships are Salesforce-managed).

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | Membership unique identifier |
| `tier_uid` | string | UID of the associated membership tier |
| `project_uid` | string | v2 UUID of the project |
| `project_slug` | string (optional) | URL slug of the project |
| `b2b_org_uid` | string (optional) | UUID of the member company |
| `status` | string | Membership status, e.g. `Active` |
| `year` | string (optional) | Membership year, e.g. `2025` |
| `tier` | string (optional) | Tier label, e.g. `Gold` |
| `auto_renew` | bool | Whether automatic renewal is enabled |
| `renewal_type` | string (optional) | Renewal cadence |
| `price` | float64 (optional) | Current membership price |
| `annual_full_price` | float64 (optional) | Full annual list price |
| `payment_frequency` | string (optional) | Payment frequency |
| `payment_terms` | string (optional) | Payment terms |
| `agreement_date` | string (optional) | Date the membership agreement was signed |
| `purchase_date` | string (optional) | Effective purchase date |
| `start_date` | string (optional) | Membership start date |
| `end_date` | string (optional) | Membership end date |
| `company_name` | string | Member company name |
| `company_logo_url` | string (optional) | Member company logo URL |
| `company_domain` | string (optional) | Member company website/domain |
| `tier_name` | string (optional) | Product name, e.g. `Gold Corporate Membership` |
| `tier_family` | string (optional) | Product family, e.g. `Membership` |
| `tier_product_type` | string (optional) | Product type |
| `created_at` | timestamp | Creation time (RFC3339) |
| `updated_at` | timestamp | Last update time (RFC3339) |

### Tags

| Tag Format | Example | Purpose |
|---|---|---|
| `{uid}` | `cbef1ed5-...` | Direct lookup by UID |
| `project_membership_uid:{uid}` | `project_membership_uid:cbef1ed5-...` | Find memberships by UID |
| `project_uid:{uid}` | `project_uid:abc-...` | Find all memberships for a project |
| `b2b_org_uid:{uid}` | `b2b_org_uid:def-...` | Find all memberships for an org |

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `project_membership:{uid}` |
| `access_check_relation` | `auditor` |
| `history_check_object` | `project_membership:{uid}` |
| `history_check_relation` | `auditor` |

### Search Behavior

| Field | Value |
|---|---|
| `fulltext` | `company_name`, `tier_name`, `status`, `year` |
| `name_and_aliases` | `company_name`, `company_domain` |
| `sort_name` | `company_name` (lowercased) |
| `public` | `false` |

### Parent References

| Ref | Condition |
|---|---|
| `b2b_org:{b2b_org_uid}` | Only when `b2b_org_uid` is set |
| `project:{project_uid}` | Only when `project_uid` is set |

---

## Key Contact

**Object type:** `key_contact`

**NATS subject:** `lfx.index.key_contact`

**Source struct:** `internal/domain/model/key_contact.go` — `KeyContact`

**Indexed on:** create, update, delete via `/project_memberships/{uid}/key_contacts` and by `/admin/reindex`.

### Data Schema

| Field | Type | Description |
|---|---|---|
| `uid` | string | Key contact unique identifier |
| `membership_uid` | string | UID of the associated project membership |
| `tier_uid` | string | UID of the associated membership tier |
| `project_uid` | string | v2 UUID of the project |
| `project_name` | string (optional) | Display name of the project — also indexed in `fulltext` for keyword search |
| `project_logo_url` | string (optional) | Logo image URL for the project |
| `b2b_org_uid` | string (optional) | UUID of the member company |
| `role` | string | Contact role, e.g. `Voting Representative` |
| `status` | string | Role record status, e.g. `Active` |
| `board_member` | bool | Whether this contact holds a board member role |
| `primary_contact` | bool | Whether this is the primary contact for the membership |
| `first_name` | string | Contact's first name |
| `last_name` | string | Contact's last name |
| `title` | string (optional) | Contact's job title |
| `email` | string (optional) | Primary email address |
| `username` | string (optional) | Resolved OIDC sub |
| `emails` | []string (optional) | Full list of email addresses |
| `company_name` | string | Member company name |
| `company_logo_url` | string (optional) | Member company logo URL |
| `company_domain` | string (optional) | Member company website/domain |
| `created_at` | timestamp | Creation time (RFC3339) |
| `updated_at` | timestamp | Last update time (RFC3339) |

### Tags

| Tag Format | Example | Purpose |
|---|---|---|
| `{uid}` | `cbef1ed5-...` | Direct lookup by UID |
| `key_contact_uid:{uid}` | `key_contact_uid:cbef1ed5-...` | Find contacts by UID |
| `project_membership_uid:{uid}` | `project_membership_uid:abc-...` | Find all contacts for a membership |
| `project_uid:{uid}` | `project_uid:def-...` | Find all contacts for a project |
| `b2b_org_uid:{uid}` | `b2b_org_uid:ghi-...` | Find all contacts for an org |
| `role:{value}` | `role:Voting Representative` | Filter contacts by role |
| `status:{value}` | `status:Active` | Filter contacts by status |

> `role` and `status` tags are only emitted when non-empty.

### Access Control (IndexingConfig)

| Field | Value |
|---|---|
| `access_check_object` | `project_membership:{membership_uid}` |
| `access_check_relation` | `auditor` |
| `history_check_object` | `project_membership:{membership_uid}` |
| `history_check_relation` | `auditor` |

### Search Behavior

| Field | Value |
|---|---|
| `fulltext` | `first_name`, `last_name`, `email`, `role`, `company_name`, `project_name` |
| `name_and_aliases` | Full name, `email` |
| `sort_name` | `last_name first_name` (lowercased) |
| `public` | `false` |
| `contacts` | `[{lfx_principal: uid, name: full_name, emails: [...]}]` |

### Parent References

| Ref | Condition |
|---|---|
| `b2b_org:{b2b_org_uid}` | Only when `b2b_org_uid` is set |
| `project:{project_uid}` | Only when `project_uid` is set |
| `project_membership:{membership_uid}` | Only when `membership_uid` is set |

---

## B2B Org Settings

**Object type:** `b2b_org_settings`

**NATS subject:** `lfx.index.b2b_org_settings`

**Source struct:** `internal/service/messaging.go` — `b2bOrgSettingsIndexerView` (adapter view; canonical state is `model.B2BOrgSettings` in the `org-settings` KV bucket)

**Trigger:** `PUT /b2b_orgs/{uid}/settings` (online path) or `POST /admin/reindex` with `types=["b2b_org_settings"]` (backfill). The doc exists only when writers/auditors have been explicitly configured via PUT.

**Action mapping:** `ActionCreated` on first write (when no prior KV record exists); `ActionUpdated` on all subsequent writes.

**Note:** `ObjectID` equals the parent org UID (not a separate settings UID) so a single point-lookup retrieves both the org doc and the settings doc. Callers filter by `object_type=b2b_org_settings`.

### Payload Fields

Flat `members[]` array — role is a first-class field on each entry. Both accepted and pending entries are included; revoked and expired are excluded.

| Field | Example value |
|---|---|
| `uid` | `cbef1ed5-...` |
| `members` | `[{username, email, name, role, invite_status, updated_at}]` |
| `created_at` | `2026-01-15T10:00:00Z` |
| `updated_at` | `2026-05-20T14:30:00Z` |

Per-member entry shape:

| Field | Example value | Notes |
|---|---|---|
| `username` | `auth0\|<lfid>` | Absent for pending invites |
| `email` | `user@example.org` | Always present |
| `name` | `Display Name` | Optional |
| `role` | `writer` | `"writer"` or `"auditor"`; writer takes precedence if user holds both |
| `invite_status` | `accepted` | `accepted`, `pending` |
| `updated_at` | `2026-01-15T10:00:00Z` | Last modification to this membership row |

### Tags

| Tag | Condition |
|---|---|
| `has_writers` | ≥1 writer with `invite_status=accepted` |
| `has_auditors` | ≥1 auditor with `invite_status=accepted` |
| `has_pending_invites` | ≥1 entry (writer or auditor) with `invite_status=pending` |
| `writer:{username}` | One tag per accepted writer with a non-empty LFID username |
| `auditor:{username}` | One tag per accepted auditor with a non-empty LFID username |
| `member:{username}` | One tag per accepted user — role-agnostic union, deduped across writer+auditor |

> Revoked and expired entries do not trigger any tag — they are audit-trail data, not actionable state.
> Pending users (no username) are included in `members[]` but produce no `writer:`, `auditor:`, or `member:` tags.

### Query Patterns

**"Which orgs is user X a member of?" (role-agnostic)**
```
GET /query/resources?v=1&type=b2b_org_settings&tags=member:auth0|{username}
```
Returns one doc per org where the user is an accepted writer or auditor. Each doc contains `data.uid` (the org UID) and the full `data.members[]` array. The `member:` tag covers both roles so a single call suffices.

**"Which orgs is user X a writer on?" (role-specific)**
```
GET /query/resources?v=1&type=b2b_org_settings&tags=writer:auth0|{username}
```

**"Which orgs is user X an auditor on?" (role-specific)**
```
GET /query/resources?v=1&type=b2b_org_settings&tags=auditor:auth0|{username}
```

**"How many orgs does user X belong to?"**
```
GET /query/resources/count?type=b2b_org_settings&tags=member:auth0|{username}
```
Returns `{"count": N, "has_more": false}`. `has_more` is true when the result exceeds the aggregation bucket limit.

**"Who has access to org Y?"**
```
GET /query/resources?v=1&type=b2b_org_settings&object_id={org_uid}
```
Returns the single settings doc for that org with the full `members[]` roster.

> All queries are enforced by an FGA `auditor` check on `b2b_org:{uid}` — only the calling user's accessible orgs are returned regardless of filter.
> Tag values containing `:` or `|` must be URL-encoded in query strings: `:` → `%3A`, `|` → `%7C`.

### Access / History Check

| Field | Value |
|---|---|
| `access_check_object` | `b2b_org:{uid}` (parent, not self — settings has no separate FGA type) |
| `access_check_relation` | `auditor` |
| `history_check_object` | `b2b_org:{uid}` |
| `history_check_relation` | `writer` (history = write-side concern; matches project-service precedent) |

### Name and Aliases / Fulltext

| Field | Contents |
|---|---|
| `name_and_aliases` | `[org.Name, org.PrimaryDomain, ...org.DomainAliases]` — domain typeahead works even with no writers configured |
| `fulltext` | Accepted writers/auditors: `Name + Email`. Pending entries: `Email + Name-if-present`. Revoked/expired excluded. |
| `sort_name` | `lower(org.Name)` |

### Parent References

| Ref | Condition |
|---|---|
| `b2b_org:{uid}` | Always (self-ref for point-lookup) |
| `b2b_org:{parent_uid}` | Only when `parent_uid` is set |

---

## NATS RPC Endpoints

