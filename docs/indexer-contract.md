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
| `access_check_relation` | `key_contact` |
| `history_check_object` | `project_membership:{membership_uid}` |
| `history_check_relation` | `auditor` |

### Search Behavior

| Field | Value |
|---|---|
| `fulltext` | `first_name`, `last_name`, `email`, `role`, `company_name` |
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
