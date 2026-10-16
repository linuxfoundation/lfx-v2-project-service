// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

const lookupKeyPrefix = "lookup/"

// Finding kinds reported by inspect children, one set per resource. A link
// without its lookup key is invisible to ListLinks; a folder or document
// without one can have its name reused.
const (
	kindMissingLookup   = "_missing_lookup"
	kindStaleLookup     = "_stale_lookup"
	kindWrongLookup     = "_wrong_lookup"
	kindDocumentNoFile  = "document_missing_file"
	kindChildUnreadable = "_unreadable_record"
	resourceLink        = "link"
	resourceFolder      = "folder"
	resourceDocument    = "document"
)

// fileStore is the part of the document object store the file check reads. It asks
// for an object's metadata only, so no file content is streamed.
type fileStore interface {
	GetInfo(ctx context.Context, name string, opts ...jetstream.GetObjectInfoOpt) (*jetstream.ObjectInfo, error)
}

type childrenSubcommand struct{}

func (s *childrenSubcommand) Name() string { return "children" }

func (s *childrenSubcommand) Help() string {
	return "check link, folder and document records against their lookup keys, and documents against their files"
}

func (s *childrenSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect children [flags]")
	skipFiles := fs.Bool("skip-files", false, "do not check that each document has its file in the object store")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}
	if err := commands.RejectExtraArgs(fs); err != nil {
		return err
	}

	natsConn, repo, err := commands.ConnectRepository(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	var files fileStore
	if !*skipFiles {
		// The repository exposes the object store through a narrower interface; the
		// jetstream store behind it also offers GetInfo.
		var ok bool
		if files, ok = repo.DocumentFiles.(fileStore); !ok {
			return fmt.Errorf("the document object store cannot report object metadata; use --skip-files")
		}
	}
	return runChildren(ctx, repo.Links, repo.Folders, repo.Documents, files, os.Stdout)
}

type childrenReport struct {
	Links     int            `json:"links"`
	Folders   int            `json:"folders"`
	Documents int            `json:"documents"`
	Counts    map[string]int `json:"counts"`
	Findings  []finding      `json:"findings"`
}

// bucketScan is one child bucket split into its records and its lookup keys.
type bucketScan struct {
	records map[string][]byte // record key (the UID) -> JSON
	lookups map[string]string // lookup key -> value
}

func scanBucket(ctx context.Context, kv projectsKV) (*bucketScan, error) {
	lister, err := kv.ListKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	scan := &bucketScan{records: map[string][]byte{}, lookups: map[string]string{}}
	for _, key := range keys {
		raw, found, err := getValue(ctx, kv, key)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		if strings.HasPrefix(key, lookupKeyPrefix) {
			scan.lookups[key] = string(raw)
		} else {
			scan.records[key] = raw
		}
	}
	return scan, nil
}

// resourceCheck says how one resource derives the lookup key its record needs.
type resourceCheck struct {
	name string
	// expect returns the record's UID and the lookup key it must have, or an
	// error when the record cannot be read.
	expect func(raw []byte) (uid, lookupKey string, err error)
	// lookupPrefix bounds the lookup keys that belong to this resource.
	lookupPrefix string
}

var (
	linkCheck = resourceCheck{
		name:         resourceLink,
		lookupPrefix: "lookup/project-links/",
		expect: func(raw []byte) (string, string, error) {
			var link models.ProjectLink
			if err := json.Unmarshal(raw, &link); err != nil {
				return "", "", err
			}
			return link.UID, fmt.Sprintf(constants.KVLookupLinkKey, link.ProjectUID, link.UID), nil
		},
	}
	folderCheck = resourceCheck{
		name:         resourceFolder,
		lookupPrefix: "lookup/project-folders/",
		expect: func(raw []byte) (string, string, error) {
			var folder models.ProjectFolder
			if err := json.Unmarshal(raw, &folder); err != nil {
				return "", "", err
			}
			return folder.UID, fmt.Sprintf(constants.KVLookupFolderPrefix, folder.BuildIndexKey(context.Background())), nil
		},
	}
	documentCheck = resourceCheck{
		name:         resourceDocument,
		lookupPrefix: "lookup/project-documents/",
		expect: func(raw []byte) (string, string, error) {
			var doc models.ProjectDocument
			if err := json.Unmarshal(raw, &doc); err != nil {
				return "", "", err
			}
			return doc.UID, fmt.Sprintf(constants.KVLookupDocumentPrefix, doc.BuildIndexKey(context.Background())), nil
		},
	}
)

// check cross-checks a bucket's records with its lookup keys and returns the
// UIDs of the readable records.
func (c resourceCheck) check(scan *bucketScan, add func(finding)) []string {
	var uids []string
	expectedKeys := make(map[string]string, len(scan.records)) // lookup key -> record UID
	for key, raw := range scan.records {
		uid, lookupKey, err := c.expect(raw)
		if err != nil || uid == "" {
			add(finding{Kind: c.name + kindChildUnreadable, UID: key, Detail: "record is not valid JSON or has no uid"})
			continue
		}
		uids = append(uids, uid)
		expectedKeys[lookupKey] = uid

		value, ok := scan.lookups[lookupKey]
		switch {
		case !ok:
			add(finding{Kind: c.name + kindMissingLookup, UID: uid, Detail: "record has no lookup key"})
		case value != uid:
			add(finding{Kind: c.name + kindWrongLookup, UID: uid, Detail: fmt.Sprintf("lookup key holds %q", value)})
		}
	}
	for key, value := range scan.lookups {
		if !strings.HasPrefix(key, c.lookupPrefix) {
			continue
		}
		if _, ok := expectedKeys[key]; !ok {
			add(finding{Kind: c.name + kindStaleLookup, UID: value, Detail: "lookup key has no matching record"})
		}
	}
	return uids
}

// runChildren checks the link, folder and document buckets. When files is nil
// the object store is not consulted.
func runChildren(ctx context.Context, links, folders, documents projectsKV, files fileStore, out io.Writer) error {
	report := &childrenReport{Counts: map[string]int{}, Findings: []finding{}}
	add := func(f finding) {
		report.Findings = append(report.Findings, f)
		report.Counts[f.Kind]++
	}

	scans := map[string]*bucketScan{}
	for name, kv := range map[string]projectsKV{resourceLink: links, resourceFolder: folders, resourceDocument: documents} {
		scan, err := scanBucket(ctx, kv)
		if err != nil {
			return fmt.Errorf("scan %s bucket: %w", name, err)
		}
		scans[name] = scan
	}

	report.Links = len(linkCheck.check(scans[resourceLink], add))
	report.Folders = len(folderCheck.check(scans[resourceFolder], add))
	documentUIDs := documentCheck.check(scans[resourceDocument], add)
	report.Documents = len(documentUIDs)

	if files != nil {
		for _, uid := range documentUIDs {
			if _, err := files.GetInfo(ctx, uid); err != nil {
				if errors.Is(err, jetstream.ErrObjectNotFound) {
					add(finding{Kind: kindDocumentNoFile, UID: uid, Detail: "no file in the object store"})
					continue
				}
				return fmt.Errorf("get document file %q: %w", uid, err)
			}
		}
	}
	sortFindings(report.Findings)

	if err := writeReport(out, report); err != nil {
		return err
	}
	return problemsError("inspect children", len(report.Findings))
}
