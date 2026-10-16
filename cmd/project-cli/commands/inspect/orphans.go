// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
)

// Finding kinds reported by inspect orphans.
const (
	kindOrphanLink     = "orphan_link"
	kindOrphanFolder   = "orphan_folder"
	kindOrphanDocument = "orphan_document"
)

// orphanRepo is the slice of the repository the orphan scan reads.
type orphanRepo interface {
	ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error)
	ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error)
	ListAllLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error)
	ListAllDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error)
}

type orphansSubcommand struct{}

func (s *orphansSubcommand) Name() string { return "orphans" }

func (s *orphansSubcommand) Help() string {
	return "list links, folders and documents whose project no longer exists"
}

func (s *orphansSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect orphans [flags]")
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

	return runOrphans(ctx, repo, os.Stdout)
}

type orphansReport struct {
	reportStatus
	Projects  int            `json:"projects"`
	Links     int            `json:"links"`
	Folders   int            `json:"folders"`
	Documents int            `json:"documents"`
	Counts    map[string]int `json:"counts"`
	Findings  []finding      `json:"findings"`
}

// runOrphans lists the child records that reference a project missing from the
// projects bucket. admin-delete-project leaves children behind on purpose, so
// these accumulate.
func runOrphans(ctx context.Context, repo orphanRepo, out io.Writer) error {
	links, err := repo.ListAllLinks(ctx, "")
	if err != nil {
		return fmt.Errorf("list links: %w", err)
	}
	folders, err := repo.ListFolders(ctx, "")
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	documents, err := repo.ListAllDocuments(ctx, "")
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}

	// The projects are read last: one created after the children were listed can
	// only be missed here, never reported as an orphan.
	bases, err := repo.ListAllProjectsBase(ctx)
	if err != nil {
		return fmt.Errorf("list project bases: %w", err)
	}
	live := make(map[string]struct{}, len(bases))
	for _, base := range bases {
		live[base.UID] = struct{}{}
	}

	report := &orphansReport{Projects: len(bases), Links: len(links), Folders: len(folders), Documents: len(documents), Counts: map[string]int{}, Findings: []finding{}}
	add := func(kind, uid, projectUID, name string) {
		if _, ok := live[projectUID]; ok {
			return
		}
		report.Findings = append(report.Findings, finding{
			Kind:   kind,
			UID:    uid,
			Detail: fmt.Sprintf("project %q not found (name %q)", projectUID, name),
		})
		report.Counts[kind]++
	}
	for _, link := range links {
		add(kindOrphanLink, link.UID, link.ProjectUID, link.Name)
	}
	for _, folder := range folders {
		add(kindOrphanFolder, folder.UID, folder.ProjectUID, folder.Name)
	}
	for _, doc := range documents {
		add(kindOrphanDocument, doc.UID, doc.ProjectUID, doc.Name)
	}
	sortFindings(report.Findings)

	report.reportStatus = statusFor(len(report.Findings))
	return finishReport(ctx, "inspect orphans", out, report, len(report.Findings), fmt.Sprintf("%d projects, %d links, %d folders, %d documents", report.Projects, report.Links, report.Folders, report.Documents))
}
