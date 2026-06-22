// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package main is an out-of-band admin utility for removing projects that the
// normal DELETE /projects/:id API refuses to delete (e.g. non-Crowdfunding
// projects that are no longer visible in the UI).
//
// For each --uid:
//  1. Reads projects/<uid>, project-settings/<uid>, and the slug-reverse-lookup
//     key from NATS KV and captures them to a JSON audit file (rollback record).
//  2. Reports any child links / folders / documents for visibility. By default
//     they are NOT deleted (left orphaned/inert once the parent is gone). With
//     --cascade-children they are deleted too (KV records, lookup keys, document
//     object-store blobs, and indexer deletes), mirroring the production
//     per-resource delete paths.
//  3. Deletes projects/<uid> with last-revision CAS (authoritative ownership check).
//     If the CAS fails (concurrent update), the run aborts before any external side-effects.
//  4. Publishes deleted-action indexer envelopes for `lfx.index.project` and
//     `lfx.index.project_settings` so the indexer service removes the docs from
//     OpenSearch (published after the CAS to avoid a search/KV inconsistency window).
//  5. Deletes projects/slug/<slug> and project-settings/<uid>.
//
// FGA cleanup (`lfx.fga-sync.delete_access`) is intentionally NOT performed by
// this script — operator has opted to let another reconciliation job handle it.
//
// Defaults to --dry-run=true. Must explicitly pass --dry-run=false to write.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	natsio "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	indexerConstants "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/constants"
	indexerTypes "github.com/linuxfoundation/lfx-v2-indexer-service/pkg/types"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/log"
	pnats "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const (
	gracefulShutdownSec = 25
	// auditTimeoutPerUID is the per-UID budget for the read-only audit phase
	// (KV reads + full child bucket scan, each key fetched individually).
	auditTimeoutPerUID = 60 * time.Second
	// executeTimeoutPerUID is the per-UID budget during execute: audit +
	// sync indexer acks (up to 10 s each × 2 messages) + KV deletes.
	executeTimeoutPerUID = 120 * time.Second
)

// uuidRE matches a canonical UUID (8-4-4-4-12 hex groups, case-insensitive).
var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// sanitizeNATSURL returns the URL with any embedded user:password redacted,
// safe to include in log output.
func sanitizeNATSURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = url.User(u.User.Username())
	return u.String()
}

// stringSliceFlag collects repeated --uid values.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	uid := strings.TrimSpace(v)
	if uid == "" {
		return errors.New("--uid cannot be empty")
	}
	if !uuidRE.MatchString(uid) {
		return fmt.Errorf("--uid %q is not a valid UUID", uid)
	}
	*s = append(*s, uid)
	return nil
}

type config struct {
	natsURL         string
	natsUser        string
	natsPassword    string
	uids            []string
	dryRun          bool
	auditPath       string
	sync            bool
	cascadeChildren bool
	skipChildScan   bool
	verbose         bool
}

func parseConfig() (config, error) {
	var cfg config
	var uids stringSliceFlag

	defaultNATS := os.Getenv("NATS_URL")
	if defaultNATS == "" {
		defaultNATS = "nats://localhost:4222"
	}

	flag.StringVar(&cfg.natsURL, "nats-url", defaultNATS, "NATS URL (env NATS_URL)")
	flag.StringVar(&cfg.natsUser, "nats-user", os.Getenv("NATS_USER"), "NATS username (env NATS_USER)")
	flag.StringVar(&cfg.natsPassword, "nats-password", os.Getenv("NATS_PASS"), "NATS password (env NATS_PASS)")
	flag.Var(&uids, "uid", "Project UID to delete (repeatable, at least one required)")
	flag.BoolVar(&cfg.dryRun, "dry-run", true, "If true (default), perform audit + plan-print only; no NATS writes. Pass --dry-run=false to execute.")
	flag.StringVar(&cfg.auditPath, "audit-file", "", "Path to write the JSON audit/backup file (default: ./admin-delete-audit-<timestamp>.json)")
	flag.BoolVar(&cfg.sync, "sync", true, "Publish indexer messages synchronously (request/reply) so we get an ack before deleting KV records")
	flag.BoolVar(&cfg.cascadeChildren, "cascade-children", false, "Also delete the project's child links, folders, and documents (KV records, lookup keys, document object-store blobs, and indexer deletes). Default false leaves children as orphaned/inert records.")
	flag.BoolVar(&cfg.skipChildScan, "skip-child-scan", false, "Skip scanning child buckets (project-links, project-folders, project-documents-metadata). Use when you have already audited children independently and know the project is a leaf record.")
	flag.BoolVar(&cfg.verbose, "verbose", false, "Verbose logging")
	flag.Parse()

	if len(uids) == 0 {
		return cfg, errors.New("at least one --uid is required")
	}
	cfg.uids = uids

	if cfg.cascadeChildren && cfg.skipChildScan {
		return cfg, errors.New("--cascade-children and --skip-child-scan are mutually exclusive")
	}

	if cfg.auditPath == "" {
		cfg.auditPath = fmt.Sprintf("./admin-delete-audit-%s.json", time.Now().UTC().Format("20060102-150405"))
	}
	return cfg, nil
}

func main() {
	log.InitStructureLogConfig()
	os.Exit(run())
}

func run() int {
	cfg, err := parseConfig()
	if err != nil {
		slog.With(constants.ErrKey, err).Error("invalid arguments")
		flag.Usage()
		return 2
	}

	slog.Info("admin-delete-project starting",
		"nats_url", sanitizeNATSURL(cfg.natsURL),
		"nats_user", cfg.natsUser,
		"uids", cfg.uids,
		"dry_run", cfg.dryRun,
		"sync_publish", cfg.sync,
		"cascade_children", cfg.cascadeChildren,
		"skip_child_scan", cfg.skipChildScan,
		"audit_file", cfg.auditPath,
	)

	timeoutPerUID := auditTimeoutPerUID
	if !cfg.dryRun {
		timeoutPerUID = executeTimeoutPerUID
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeoutPerUID*time.Duration(len(cfg.uids)+1))
	defer cancel()

	natsOpts := []natsio.Option{
		natsio.DrainTimeout(gracefulShutdownSec * time.Second),
		natsio.ConnectHandler(func(_ *natsio.Conn) {
			slog.With("nats_url", cfg.natsURL).Info("NATS connection established")
		}),
		natsio.ErrorHandler(func(_ *natsio.Conn, s *natsio.Subscription, e error) {
			if s != nil {
				slog.With(constants.ErrKey, e, "subject", s.Subject, "queue", s.Queue).Error("async NATS error")
			} else {
				slog.With(constants.ErrKey, e).Error("async NATS error outside subscription")
			}
		}),
	}
	if cfg.natsUser != "" {
		natsOpts = append(natsOpts, natsio.UserInfo(cfg.natsUser, cfg.natsPassword))
	}

	nc, err := natsio.Connect(cfg.natsURL, natsOpts...)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("failed to connect to NATS")
		return 1
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("failed to create JetStream client")
		return 1
	}

	needChildBuckets := !cfg.skipChildScan || cfg.cascadeChildren
	buckets, err := openBuckets(ctx, js, needChildBuckets)
	if err != nil {
		slog.With(constants.ErrKey, err).Error("failed to open required NATS KV buckets")
		return 1
	}

	mb := &pnats.MessageBuilder{NatsConn: nc}

	auditRecords := make([]projectAudit, 0, len(cfg.uids))
	hadFailure := false

	for _, uid := range cfg.uids {
		record, err := auditProject(ctx, buckets, uid, cfg.skipChildScan)
		if err != nil {
			slog.With(constants.ErrKey, err, "uid", uid).Error("audit failed; aborting this UID")
			hadFailure = true
			continue
		}
		auditRecords = append(auditRecords, record)
	}

	if err := writeAudit(cfg.auditPath, auditRecords); err != nil {
		slog.With(constants.ErrKey, err, "audit_file", cfg.auditPath).Error("failed to write audit file")
		return 1
	}
	slog.With("audit_file", cfg.auditPath, "records", len(auditRecords)).Info("audit file written")

	// Child links/folders/documents are reported for visibility. With
	// --cascade-children they are deleted too; otherwise they are left in place
	// as orphaned/inert records (their access checks resolve against a project
	// object that no longer exists).
	for _, rec := range auditRecords {
		if rec.Children.Found {
			disposition := "they will be LEFT in place as orphaned/inert records (use --cascade-children to delete them)"
			if cfg.cascadeChildren {
				disposition = "they WILL be cascade-deleted (--cascade-children enabled)"
			}
			slog.With(
				"uid", rec.UID,
				"link_count", len(rec.Children.Links),
				"folder_count", len(rec.Children.Folders),
				"document_count", len(rec.Children.Documents),
				"link_lookup_keys", rec.Children.LinkLookupKeys,
				"folder_uids", rec.Children.FolderUIDs,
				"document_uids", rec.Children.DocumentUIDs,
			).Warn("project has child resources; " + disposition)
		}
	}
	if hadFailure {
		slog.Error("aborting before any writes due to audit read failures above")
		return 1
	}

	if cfg.dryRun {
		printPlan(auditRecords, cfg.cascadeChildren)
		slog.Info("dry-run mode (default); no writes performed. Re-run with --dry-run=false to execute.")
		return 0
	}

	exitCode := 0
	for _, rec := range auditRecords {
		if err := executeDelete(ctx, buckets, mb, rec, cfg.sync, cfg.cascadeChildren); err != nil {
			slog.With(constants.ErrKey, err, "uid", rec.UID).Error("delete failed for UID")
			exitCode = 1
			continue
		}
		slog.With("uid", rec.UID, "slug", rec.Slug).Info("project deleted")
	}
	if exitCode != 0 {
		slog.Error("one or more UIDs failed to delete completely; see audit file for state")
	} else {
		slog.Info("admin-delete-project completed successfully")
	}
	return exitCode
}

// ─── Bucket handles ──────────────────────────────────────────────────────────

type kvBuckets struct {
	Projects        jetstream.KeyValue
	ProjectSettings jetstream.KeyValue
	Links           jetstream.KeyValue    // optional
	Folders         jetstream.KeyValue    // optional
	Documents       jetstream.KeyValue    // optional
	DocumentFiles   jetstream.ObjectStore // optional (document file blobs)
}

// openBuckets opens the required project KV buckets. Child buckets
// (links/folders/documents) are only opened when needChildBuckets is true —
// i.e. when the child scan will actually run or cascade is enabled.
// Skipping them avoids slow/blocking handshakes (especially the object store)
// when the caller already knows children are not relevant.
func openBuckets(ctx context.Context, js jetstream.JetStream, needChildBuckets bool) (kvBuckets, error) {
	var kv kvBuckets
	var err error

	if kv.Projects, err = js.KeyValue(ctx, constants.KVStoreNameProjects); err != nil {
		return kv, fmt.Errorf("open bucket %s: %w", constants.KVStoreNameProjects, err)
	}
	if kv.ProjectSettings, err = js.KeyValue(ctx, constants.KVStoreNameProjectSettings); err != nil {
		return kv, fmt.Errorf("open bucket %s: %w", constants.KVStoreNameProjectSettings, err)
	}

	if !needChildBuckets {
		slog.Info("child bucket handles skipped (child scan and cascade both disabled)")
		return kv, nil
	}

	// Optional child buckets — only opened when scan or cascade is needed.
	if kv.Links, err = js.KeyValue(ctx, constants.KVStoreNameProjectLinks); err != nil {
		slog.With(constants.ErrKey, err, "bucket", constants.KVStoreNameProjectLinks).Warn("optional KV bucket not present; child scan will skip it")
		kv.Links = nil
	}
	if kv.Folders, err = js.KeyValue(ctx, constants.KVStoreNameProjectFolders); err != nil {
		slog.With(constants.ErrKey, err, "bucket", constants.KVStoreNameProjectFolders).Warn("optional KV bucket not present; child scan will skip it")
		kv.Folders = nil
	}
	if kv.Documents, err = js.KeyValue(ctx, constants.KVStoreNameProjectDocuments); err != nil {
		slog.With(constants.ErrKey, err, "bucket", constants.KVStoreNameProjectDocuments).Warn("optional KV bucket not present; child scan will skip it")
		kv.Documents = nil
	}
	if kv.DocumentFiles, err = js.ObjectStore(ctx, constants.ObjectStoreNameProjectDocuments); err != nil {
		slog.With(constants.ErrKey, err, "store", constants.ObjectStoreNameProjectDocuments).Warn("optional object store not present; cascade will skip document blob deletes")
		kv.DocumentFiles = nil
	}
	return kv, nil
}

// ─── Audit ───────────────────────────────────────────────────────────────────

// projectAudit captures everything we read about a project before any write.
// Written to the audit JSON file so the operator has a rollback reference.
type projectAudit struct {
	UID      string           `json:"uid"`
	Base     auditedKVEntry   `json:"base"`           // projects/<uid>
	BaseObj  map[string]any   `json:"base_object"`    // parsed JSON for readability
	Slug     string           `json:"slug,omitempty"` // derived from base
	SlugKV   auditedKVEntry   `json:"slug_kv"`        // projects/slug/<slug>
	Settings auditedKVEntry   `json:"settings"`       // project-settings/<uid>
	Children childAuditResult `json:"children"`
	AuditAt  time.Time        `json:"audited_at"`
	Errors   []string         `json:"errors,omitempty"`
}

type auditedKVEntry struct {
	Key      string `json:"key"`
	Found    bool   `json:"found"`
	Revision uint64 `json:"revision,omitempty"`
	// Raw value (base64-decoded by json.Marshal automatically for []byte).
	Value []byte `json:"value,omitempty"`
}

type childAuditResult struct {
	Found          bool                     `json:"found"`
	LinkLookupKeys []string                 `json:"link_lookup_keys,omitempty"`
	FolderUIDs     []string                 `json:"folder_uids,omitempty"`
	DocumentUIDs   []string                 `json:"document_uids,omitempty"`
	Links          []models.ProjectLink     `json:"-"` // full records for cascade; omitted from audit JSON
	Folders        []models.ProjectFolder   `json:"-"`
	Documents      []models.ProjectDocument `json:"-"`
}

// extractSlugFromBase parses the raw NATS KV bytes of a project base record and
// extracts the slug field. Returns the slug (empty string if absent or non-string),
// the full parsed map, and any JSON unmarshal error.
func extractSlugFromBase(data []byte) (slug string, baseObj map[string]any, err error) {
	if err = json.Unmarshal(data, &baseObj); err != nil {
		return "", nil, err
	}
	if s, ok := baseObj["slug"].(string); ok {
		slug = s
	}
	return slug, baseObj, nil
}

func auditProject(ctx context.Context, kv kvBuckets, uid string, skipChildScan bool) (projectAudit, error) {
	rec := projectAudit{UID: uid, AuditAt: time.Now().UTC()}

	// projects/<uid>
	rec.Base.Key = uid
	baseEntry, err := kv.Projects.Get(ctx, uid)
	switch {
	case err == nil:
		rec.Base.Found = true
		rec.Base.Revision = baseEntry.Revision()
		rec.Base.Value = baseEntry.Value()
		// Parse just enough to extract the slug and to dump as readable JSON.
		if slug, baseObj, uerr := extractSlugFromBase(baseEntry.Value()); uerr != nil {
			rec.Errors = append(rec.Errors, fmt.Sprintf("base unmarshal: %v", uerr))
		} else {
			rec.Slug = slug
			rec.BaseObj = baseObj
		}
	case errors.Is(err, jetstream.ErrKeyNotFound):
		// Not present; the project may have been partially cleaned up already.
		slog.With("uid", uid).Warn("project base not found in projects KV; nothing to delete from base bucket")
	default:
		return rec, fmt.Errorf("read projects/%s: %w", uid, err)
	}

	// projects/slug/<slug>
	if rec.Slug != "" {
		slugKey := fmt.Sprintf("slug/%s", rec.Slug)
		rec.SlugKV.Key = slugKey
		slugEntry, err := kv.Projects.Get(ctx, slugKey)
		switch {
		case err == nil:
			rec.SlugKV.Found = true
			rec.SlugKV.Revision = slugEntry.Revision()
			rec.SlugKV.Value = slugEntry.Value()
		case errors.Is(err, jetstream.ErrKeyNotFound):
			slog.With("uid", uid, "slug_key", slugKey).Warn("slug mapping not found")
		default:
			return rec, fmt.Errorf("read projects/%s: %w", slugKey, err)
		}
	}

	// project-settings/<uid>
	rec.Settings.Key = uid
	settingsEntry, err := kv.ProjectSettings.Get(ctx, uid)
	switch {
	case err == nil:
		rec.Settings.Found = true
		rec.Settings.Revision = settingsEntry.Revision()
		rec.Settings.Value = settingsEntry.Value()
	case errors.Is(err, jetstream.ErrKeyNotFound):
		slog.With("uid", uid).Warn("project settings not found")
	default:
		return rec, fmt.Errorf("read project-settings/%s: %w", uid, err)
	}

	// Children — skipped when --skip-child-scan is set.
	if skipChildScan {
		slog.With("uid", uid).Info("child scan skipped (--skip-child-scan); children assumed absent and will not be touched")
	} else {
		children, err := auditChildren(ctx, kv, uid)
		if err != nil {
			return rec, fmt.Errorf("audit children: %w", err)
		}
		rec.Children = children
	}

	return rec, nil
}

// auditChildren scans the optional child KV buckets for any record referencing
// this project. It captures both summary IDs (for the audit JSON) and the full
// typed records (used by the cascade phase). Returns Found=true if any child
// exists.
func auditChildren(ctx context.Context, kv kvBuckets, uid string) (childAuditResult, error) {
	res := childAuditResult{}

	if kv.Links != nil {
		links, lookupKeys, err := scanLinks(ctx, kv.Links, uid)
		if err != nil {
			return res, fmt.Errorf("scan project-links: %w", err)
		}
		res.Links = links
		res.LinkLookupKeys = lookupKeys
		for i := range links {
			key := fmt.Sprintf(constants.KVLookupLinkKey, links[i].ProjectUID, links[i].UID)
			if !slices.Contains(res.LinkLookupKeys, key) {
				res.LinkLookupKeys = append(res.LinkLookupKeys, key)
			}
		}
	}

	if kv.Folders != nil {
		folders, err := scanFolders(ctx, kv.Folders, uid)
		if err != nil {
			return res, fmt.Errorf("scan project-folders: %w", err)
		}
		res.Folders = folders
		for i := range folders {
			res.FolderUIDs = append(res.FolderUIDs, folders[i].UID)
		}
	}

	if kv.Documents != nil {
		docs, err := scanDocuments(ctx, kv.Documents, uid)
		if err != nil {
			return res, fmt.Errorf("scan project-documents-metadata: %w", err)
		}
		res.Documents = docs
		for i := range docs {
			res.DocumentUIDs = append(res.DocumentUIDs, docs[i].UID)
		}
	}

	res.Found = len(res.Links) > 0 || len(res.Folders) > 0 || len(res.Documents) > 0 || len(res.LinkLookupKeys) > 0
	return res, nil
}

func listAllKeys(ctx context.Context, kv jetstream.KeyValue) ([]string, error) {
	lister, err := kv.ListKeys(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = lister.Stop() }()

	var keys []string
	for {
		select {
		case k, ok := <-lister.Keys():
			if !ok {
				return keys, nil
			}
			keys = append(keys, k)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// scanLinks returns all link records whose project_uid == uid, plus any
// lookup/project-links/<uid>/... keys present in the bucket.
func scanLinks(ctx context.Context, kv jetstream.KeyValue, uid string) ([]models.ProjectLink, []string, error) {
	keys, err := listAllKeys(ctx, kv)
	if err != nil {
		return nil, nil, err
	}
	lookupPrefix := fmt.Sprintf("lookup/project-links/%s/", uid)
	var links []models.ProjectLink
	var lookupKeys []string
	for _, k := range keys {
		if strings.HasPrefix(k, "lookup/") {
			if strings.HasPrefix(k, lookupPrefix) {
				lookupKeys = append(lookupKeys, k)
			}
			continue
		}
		entry, err := kv.Get(ctx, k)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, nil, fmt.Errorf("get key %q: %w", k, err)
		}
		var l models.ProjectLink
		if uerr := json.Unmarshal(entry.Value(), &l); uerr != nil {
			slog.With(constants.ErrKey, uerr, "key", k).Warn("could not unmarshal link record while scanning; skipping")
			continue
		}
		if l.ProjectUID == uid {
			links = append(links, l)
		}
	}
	return links, lookupKeys, nil
}

func scanFolders(ctx context.Context, kv jetstream.KeyValue, uid string) ([]models.ProjectFolder, error) {
	keys, err := listAllKeys(ctx, kv)
	if err != nil {
		return nil, err
	}
	var folders []models.ProjectFolder
	for _, k := range keys {
		if strings.HasPrefix(k, "lookup/") {
			continue
		}
		entry, err := kv.Get(ctx, k)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("get key %q: %w", k, err)
		}
		var f models.ProjectFolder
		if uerr := json.Unmarshal(entry.Value(), &f); uerr != nil {
			slog.With(constants.ErrKey, uerr, "key", k).Warn("could not unmarshal folder record while scanning; skipping")
			continue
		}
		if f.ProjectUID == uid {
			folders = append(folders, f)
		}
	}
	return folders, nil
}

func scanDocuments(ctx context.Context, kv jetstream.KeyValue, uid string) ([]models.ProjectDocument, error) {
	keys, err := listAllKeys(ctx, kv)
	if err != nil {
		return nil, err
	}
	var docs []models.ProjectDocument
	for _, k := range keys {
		if strings.HasPrefix(k, "lookup/") {
			continue
		}
		entry, err := kv.Get(ctx, k)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("get key %q: %w", k, err)
		}
		var d models.ProjectDocument
		if uerr := json.Unmarshal(entry.Value(), &d); uerr != nil {
			slog.With(constants.ErrKey, uerr, "key", k).Warn("could not unmarshal document record while scanning; skipping")
			continue
		}
		if d.ProjectUID == uid {
			docs = append(docs, d)
		}
	}
	return docs, nil
}

func writeAudit(path string, records []projectAudit) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}

// ─── Plan / Execute ──────────────────────────────────────────────────────────

func printPlan(records []projectAudit, cascade bool) {
	for _, rec := range records {
		slog.With(
			"uid", rec.UID,
			"slug", rec.Slug,
			"base_found", rec.Base.Found,
			"base_revision", rec.Base.Revision,
			"slug_kv_found", rec.SlugKV.Found,
			"settings_found", rec.Settings.Found,
			"cascade_children", cascade,
			"child_links", len(rec.Children.Links),
			"child_folders", len(rec.Children.Folders),
			"child_documents", len(rec.Children.Documents),
		).Info("plan: would publish indexer deletes and delete KV records")
	}
}

// executeDelete performs the write phase for a single project in the following order:
//  1. Cascade children (when requested), so orphaned children are removed before the parent.
//  2. KV CAS delete of the base record — this is the authoritative ownership check.
//     If a concurrent update has bumped the revision, we abort here before touching anything else.
//  3. Indexer deletes for project + settings (after KV ownership is confirmed).
//  4. Slug reverse-lookup key and settings KV cleanup.
//
// Doing the CAS first eliminates the consistency window where the project is removed from
// OpenSearch but still live in NATS KV (the window that exists when indexer is published first).
func executeDelete(ctx context.Context, kv kvBuckets, mb *pnats.MessageBuilder, rec projectAudit, sync, cascade bool) error {
	uid := rec.UID

	// 0. Cascade children first, so that if the parent delete later fails we have
	// not left the children pointing at a still-present project in an odd state.
	if cascade && rec.Children.Found {
		if err := cascadeDeleteChildren(ctx, kv, mb, rec, sync); err != nil {
			return fmt.Errorf("cascade children: %w", err)
		}
	}

	// 1. KV CAS delete — confirm ownership before any external side-effects.
	// If the revision has changed since audit (concurrent update), abort cleanly.
	if rec.Base.Found {
		if err := kv.Projects.Delete(ctx, uid, jetstream.LastRevision(rec.Base.Revision)); err != nil {
			return fmt.Errorf("delete projects/%s (rev %d): %w", uid, rec.Base.Revision, err)
		}
		slog.With("uid", uid, "revision", rec.Base.Revision).Info("deleted projects/<uid>")
	}

	// 2. Indexer: project + project_settings.
	// Published after the CAS succeeds so there is no window where the project is
	// absent from OpenSearch but still present in NATS KV.
	// Passing the UID as a string tells SendIndexerMessage to construct an
	// ActionDeleted envelope (see internal/infrastructure/nats/message.go).
	if err := mb.SendIndexerMessage(ctx, constants.IndexProjectSubject, uid, sync); err != nil {
		return fmt.Errorf("publish %s deleted: %w", constants.IndexProjectSubject, err)
	}
	slog.With("uid", uid, "subject", constants.IndexProjectSubject).Info("published indexer delete")

	if err := mb.SendIndexerMessage(ctx, constants.IndexProjectSettingsSubject, uid, sync); err != nil {
		return fmt.Errorf("publish %s deleted: %w", constants.IndexProjectSettingsSubject, err)
	}
	slog.With("uid", uid, "subject", constants.IndexProjectSettingsSubject).Info("published indexer delete")

	// FGA cleanup intentionally skipped per operator policy.

	if rec.SlugKV.Found {
		// Slug mapping is single-writer; CAS not required.
		if err := kv.Projects.Delete(ctx, rec.SlugKV.Key); err != nil {
			return fmt.Errorf("delete projects/%s: %w", rec.SlugKV.Key, err)
		}
		slog.With("uid", uid, "slug_key", rec.SlugKV.Key).Info("deleted slug reverse-lookup")
	} else if rec.Base.Found && rec.Slug == "" && len(rec.Errors) > 0 {
		// Base record was malformed — slug could not be extracted during audit,
		// so the slug/* reverse-lookup key was not deleted. Manual cleanup required.
		slog.With("uid", uid, "audit_errors", rec.Errors).Warn("slug reverse-lookup key NOT deleted: base JSON was malformed during audit; locate and delete the slug/* key manually")
	}

	if rec.Settings.Found {
		if err := kv.ProjectSettings.Delete(ctx, uid); err != nil {
			return fmt.Errorf("delete project-settings/%s: %w", uid, err)
		}
		slog.With("uid", uid).Info("deleted project-settings/<uid>")
	}

	return nil
}

// cascadeDeleteChildren deletes all child links, folders, and documents for the
// project, mirroring the per-resource production delete paths: publish an
// indexer delete (built from the same IndexingConfig the service uses), delete
// the KV record, purge the lookup/uniqueness key, and for documents delete the
// object-store blob. Each child is processed independently; the first error
// aborts and is returned.
func cascadeDeleteChildren(ctx context.Context, kv kvBuckets, mb *pnats.MessageBuilder, rec projectAudit, sync bool) error {
	uid := rec.UID

	// Links
	for i := range rec.Children.Links {
		l := rec.Children.Links[i]
		msg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionDeleted,
			Data:           l.UID,
			IndexingConfig: l.IndexingConfig(),
		}
		if err := mb.SendIndexerMessage(ctx, constants.IndexProjectLinkSubject, msg, sync); err != nil {
			return fmt.Errorf("publish link %s deleted: %w", l.UID, err)
		}
		if kv.Links != nil {
			if err := kv.Links.Delete(ctx, l.UID); err != nil {
				return fmt.Errorf("delete project-links/%s: %w", l.UID, err)
			}
			lookupKey := fmt.Sprintf(constants.KVLookupLinkKey, l.ProjectUID, l.UID)
			if err := kv.Links.Purge(ctx, lookupKey); err != nil {
				slog.With(constants.ErrKey, err, "key", lookupKey).Warn("could not purge link lookup key; continuing")
			}
		}
		slog.With("uid", uid, "link_uid", l.UID).Info("cascade-deleted link")
	}

	// Folders
	for i := range rec.Children.Folders {
		f := rec.Children.Folders[i]
		msg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionDeleted,
			Data:           f.UID,
			IndexingConfig: f.IndexingConfig(),
		}
		if err := mb.SendIndexerMessage(ctx, constants.IndexProjectFolderSubject, msg, sync); err != nil {
			return fmt.Errorf("publish folder %s deleted: %w", f.UID, err)
		}
		if kv.Folders != nil {
			if err := kv.Folders.Delete(ctx, f.UID); err != nil {
				return fmt.Errorf("delete project-folders/%s: %w", f.UID, err)
			}
			uniqueKey := fmt.Sprintf(constants.KVLookupFolderPrefix, f.BuildIndexKey(ctx))
			if err := kv.Folders.Purge(ctx, uniqueKey); err != nil {
				slog.With(constants.ErrKey, err, "key", uniqueKey).Warn("could not purge folder lookup key; continuing")
			}
		}
		slog.With("uid", uid, "folder_uid", f.UID).Info("cascade-deleted folder")
	}

	// Documents (metadata KV + lookup key + object-store blob)
	for i := range rec.Children.Documents {
		d := rec.Children.Documents[i]
		msg := indexerTypes.IndexerMessageEnvelope{
			Action:         indexerConstants.ActionDeleted,
			Data:           d.UID,
			IndexingConfig: d.IndexingConfig(),
		}
		if err := mb.SendIndexerMessage(ctx, constants.IndexProjectDocumentSubject, msg, sync); err != nil {
			return fmt.Errorf("publish document %s deleted: %w", d.UID, err)
		}
		if kv.Documents != nil {
			if err := kv.Documents.Delete(ctx, d.UID); err != nil {
				return fmt.Errorf("delete project-documents-metadata/%s: %w", d.UID, err)
			}
			uniqueKey := fmt.Sprintf(constants.KVLookupDocumentPrefix, d.BuildIndexKey(ctx))
			if err := kv.Documents.Purge(ctx, uniqueKey); err != nil {
				slog.With(constants.ErrKey, err, "key", uniqueKey).Warn("could not purge document lookup key; continuing")
			}
		}
		if kv.DocumentFiles != nil {
			if err := kv.DocumentFiles.Delete(ctx, d.UID); err != nil {
				slog.With(constants.ErrKey, err, "document_uid", d.UID).Warn("could not delete document blob from object store; continuing")
			}
		}
		slog.With("uid", uid, "document_uid", d.UID).Info("cascade-deleted document")
	}

	return nil
}
