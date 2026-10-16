// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"context"
	"fmt"
	"io"
	"os"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/osdiff"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// Finding kinds reported by inspect index-drift.
const (
	kindMissingProjectDoc  = "missing_project_document"
	kindMissingSettingsDoc = "missing_settings_document"
)

// projectLister is the slice of the repository the drift check reads.
type projectLister interface {
	ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error)
}

type indexDriftSubcommand struct{}

func (s *indexDriftSubcommand) Name() string { return "index-drift" }

func (s *indexDriftSubcommand) Help() string {
	return "list projects whose project or project_settings OpenSearch documents are missing"
}

func (s *indexDriftSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect index-drift [flags]")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}
	if err := commands.RejectExtraArgs(fs); err != nil {
		return err
	}

	osClient, err := osinfra.NewClient(ctx, rc.OpenSearchConfig)
	if err != nil {
		return err
	}
	natsConn, repo, err := commands.ConnectRepository(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	return runIndexDrift(ctx, repo, osClient, os.Stdout)
}

type indexDriftReport struct {
	Projects int            `json:"projects"`
	Counts   map[string]int `json:"counts"`
	Findings []finding      `json:"findings"`
}

// runIndexDrift reports the diff reindex-projects would act on, without
// publishing anything. The hidden ROOT project is never indexed, so it is left
// out, as reindex-projects does.
func runIndexDrift(ctx context.Context, repo projectLister, client *opensearchgo.Client, out io.Writer) error {
	all, err := repo.ListAllProjectsBase(ctx)
	if err != nil {
		return fmt.Errorf("list project bases: %w", err)
	}
	bases := make([]*models.ProjectBase, 0, len(all))
	for _, base := range all {
		if base.Slug != constants.RootProjectSlug {
			bases = append(bases, base)
		}
	}

	missing, err := osdiff.Diff(ctx, client, bases)
	if err != nil {
		return fmt.Errorf("diff opensearch: %w", err)
	}

	report := &indexDriftReport{Projects: len(bases), Counts: map[string]int{}, Findings: []finding{}}
	for _, base := range bases {
		m := missing[base.UID]
		if m.Project {
			report.Findings = append(report.Findings, finding{Kind: kindMissingProjectDoc, Slug: base.Slug, UID: base.UID})
			report.Counts[kindMissingProjectDoc]++
		}
		if m.Settings {
			report.Findings = append(report.Findings, finding{Kind: kindMissingSettingsDoc, Slug: base.Slug, UID: base.UID})
			report.Counts[kindMissingSettingsDoc]++
		}
	}
	sortFindings(report.Findings)

	if err := writeReport(out, report); err != nil {
		return err
	}
	return problemsError("inspect index-drift", len(report.Findings))
}
