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
	"sort"
	"strings"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

const slugKeyPrefix = "slug/"

// Finding kinds reported by inspect slug-index.
const (
	kindDanglingIndex   = "dangling_index"
	kindMismatchedIndex = "mismatched_index"
	kindMissingIndex    = "missing_index"
	kindWrongIndex      = "wrong_index"
	kindUnreadable      = "unreadable_record"
)

// projectsKV is the part of the projects bucket the slug check reads.
type projectsKV interface {
	ListKeys(ctx context.Context, opts ...jetstream.WatchOpt) (jetstream.KeyLister, error)
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
}

type slugIndexSubcommand struct{}

func (s *slugIndexSubcommand) Name() string { return "slug-index" }

func (s *slugIndexSubcommand) Help() string {
	return "check every slug/<slug> index entry against its project record, in both directions"
}

func (s *slugIndexSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect slug-index [flags]")
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

	return runSlugIndex(ctx, repo.Projects, os.Stdout)
}

type slugIndexReport struct {
	reportStatus
	Projects    int            `json:"projects"`
	SlugEntries int            `json:"slug_entries"`
	Counts      map[string]int `json:"counts"`
	Findings    []finding      `json:"findings"`
}

// runSlugIndex reads the projects bucket once and cross-checks the slug index
// against the project records, reporting what the rename-project-slug repair
// would otherwise have to find.
func runSlugIndex(ctx context.Context, kv projectsKV, out io.Writer) error {
	lister, err := kv.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("list project keys: %w", err)
	}

	var slugKeys, projectKeys []string
	for key := range lister.Keys() {
		if strings.HasPrefix(key, slugKeyPrefix) {
			slugKeys = append(slugKeys, key)
		} else {
			projectKeys = append(projectKeys, key)
		}
	}

	report := &slugIndexReport{Counts: map[string]int{}, Findings: []finding{}}
	add := func(f finding) {
		report.Findings = append(report.Findings, f)
		report.Counts[f.Kind]++
	}

	// uid -> slug from the records, slug -> uid from the index.
	slugByUID := make(map[string]string, len(projectKeys))
	for _, key := range projectKeys {
		raw, found, err := getValue(ctx, kv, key)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		var base models.ProjectBase
		if err := json.Unmarshal(raw, &base); err != nil {
			add(finding{Kind: kindUnreadable, UID: key, Detail: "project record is not valid JSON"})
			continue
		}
		slugByUID[key] = base.Slug
	}
	report.Projects = len(slugByUID)

	uidBySlug := make(map[string]string, len(slugKeys))
	for _, key := range slugKeys {
		raw, found, err := getValue(ctx, kv, key)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		uidBySlug[strings.TrimPrefix(key, slugKeyPrefix)] = string(raw)
	}
	report.SlugEntries = len(uidBySlug)

	for slug, uid := range uidBySlug {
		recordSlug, exists := slugByUID[uid]
		switch {
		case !exists:
			add(finding{Kind: kindDanglingIndex, Slug: slug, UID: uid, Detail: "index points at a UID with no project record"})
		case recordSlug != slug:
			add(finding{Kind: kindMismatchedIndex, Slug: slug, UID: uid, Detail: fmt.Sprintf("project record has slug %q", recordSlug)})
		}
	}
	for uid, slug := range slugByUID {
		indexed, exists := uidBySlug[slug]
		switch {
		case !exists:
			add(finding{Kind: kindMissingIndex, Slug: slug, UID: uid, Detail: "no slug index entry for this project"})
		case indexed != uid:
			add(finding{Kind: kindWrongIndex, Slug: slug, UID: uid, Detail: fmt.Sprintf("slug index points at %q", indexed)})
		}
	}
	sortFindings(report.Findings)

	report.reportStatus = statusFor(len(report.Findings))
	return finishReport(ctx, "inspect slug-index", out, report, len(report.Findings), fmt.Sprintf("%d projects, %d slug entries", report.Projects, report.SlugEntries))
}

// getValue reads one key. A key that vanished between the listing and the read
// was deleted in the meantime, so it is reported as not found rather than failing.
func getValue(ctx context.Context, kv projectsKV, key string) ([]byte, bool, error) {
	entry, err := kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get %q: %w", key, err)
	}
	return entry.Value(), true, nil
}

func sortFindings(findings []finding) {
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Slug != b.Slug {
			return a.Slug < b.Slug
		}
		return a.UID < b.UID
	})
}
