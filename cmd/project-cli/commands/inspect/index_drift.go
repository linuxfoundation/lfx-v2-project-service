// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/osdiff"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// Resources the drift check can cover. "project" covers the project and
// project_settings documents.
const (
	driftProject  = "project"
	driftLink     = "link"
	driftFolder   = "folder"
	driftDocument = "document"
)

var driftResources = []string{driftProject, driftLink, driftFolder, driftDocument}

// driftRepo is the slice of the repository the drift check reads.
type driftRepo interface {
	ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error)
	ListAllLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error)
	ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error)
	ListAllDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error)
}

type indexDriftSubcommand struct{}

func (s *indexDriftSubcommand) Name() string { return "index-drift" }

func (s *indexDriftSubcommand) Help() string {
	return "list project, link, folder and document records whose OpenSearch document is missing"
}

func (s *indexDriftSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect index-drift [flags]")
	resource := fs.String("resource", "all", "comma-separated resources to check: "+strings.Join(driftResources, ", ")+", or all")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}
	if err := commands.RejectExtraArgs(fs); err != nil {
		return err
	}
	selected, err := parseDriftResources(*resource)
	if err != nil {
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

	return runIndexDrift(ctx, repo, osClient, selected, os.Stdout)
}

// parseDriftResources turns the --resource value into a set. "all" selects every resource.
func parseDriftResources(value string) (map[string]bool, error) {
	selected := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "all":
			for _, resource := range driftResources {
				selected[resource] = true
			}
		case driftProject, driftLink, driftFolder, driftDocument:
			selected[name] = true
		default:
			return nil, fmt.Errorf("unknown resource %q: use %s, or all", name, strings.Join(driftResources, ", "))
		}
	}
	return selected, nil
}

type indexDriftReport struct {
	// Checked is how many documents were looked up, per document type.
	Checked  map[string]int `json:"checked"`
	Counts   map[string]int `json:"counts"`
	Findings []finding      `json:"findings"`
}

// expectedDoc is a search document a KV record calls for, with what to say if it is absent.
type expectedDoc struct {
	osdiff.Doc
	slug   string
	detail string
}

// runIndexDrift reports the documents the resources index lacks, without publishing
// anything. The project diff is the one reindex-projects acts on; the hidden ROOT
// project is never indexed, so it is left out, as reindex-projects does.
func runIndexDrift(ctx context.Context, repo driftRepo, client *opensearchgo.Client, selected map[string]bool, out io.Writer) error {
	var expected []expectedDoc

	if selected[driftProject] {
		bases, err := repo.ListAllProjectsBase(ctx)
		if err != nil {
			return fmt.Errorf("list project bases: %w", err)
		}
		for _, base := range bases {
			if base.Slug == constants.RootProjectSlug {
				continue
			}
			for _, docType := range []string{osdiff.TypeProject, osdiff.TypeProjectSettings} {
				expected = append(expected, expectedDoc{
					Doc:    osdiff.Doc{Type: docType, UID: base.UID},
					slug:   base.Slug,
					detail: "no " + docType + " document in the resources index",
				})
			}
		}
	}
	if selected[driftLink] {
		links, err := repo.ListAllLinks(ctx, "")
		if err != nil {
			return fmt.Errorf("list links: %w", err)
		}
		for _, link := range links {
			expected = append(expected, childDoc(osdiff.TypeProjectLink, link.UID, link.ProjectUID))
		}
	}
	if selected[driftFolder] {
		folders, err := repo.ListFolders(ctx, "")
		if err != nil {
			return fmt.Errorf("list folders: %w", err)
		}
		for _, folder := range folders {
			expected = append(expected, childDoc(osdiff.TypeProjectFolder, folder.UID, folder.ProjectUID))
		}
	}
	if selected[driftDocument] {
		documents, err := repo.ListAllDocuments(ctx, "")
		if err != nil {
			return fmt.Errorf("list documents: %w", err)
		}
		for _, doc := range documents {
			expected = append(expected, childDoc(osdiff.TypeProjectDocument, doc.UID, doc.ProjectUID))
		}
	}

	docs := make([]osdiff.Doc, len(expected))
	report := &indexDriftReport{Checked: map[string]int{}, Counts: map[string]int{}, Findings: []finding{}}
	for i, e := range expected {
		docs[i] = e.Doc
		report.Checked[e.Type]++
	}
	missing, err := osdiff.MissingDocs(ctx, client, docs)
	if err != nil {
		return fmt.Errorf("diff opensearch: %w", err)
	}

	byDoc := make(map[osdiff.Doc]expectedDoc, len(expected))
	for _, e := range expected {
		byDoc[e.Doc] = e
	}
	for _, doc := range missing {
		e := byDoc[doc]
		kind := "missing_" + doc.Type
		report.Findings = append(report.Findings, finding{Kind: kind, Slug: e.slug, UID: doc.UID, Detail: e.detail})
		report.Counts[kind]++
	}
	sortFindings(report.Findings)

	if err := writeReport(out, report); err != nil {
		return err
	}
	return problemsError("inspect index-drift", len(report.Findings))
}

func childDoc(docType, uid, projectUID string) expectedDoc {
	return expectedDoc{
		Doc:    osdiff.Doc{Type: docType, UID: uid},
		detail: fmt.Sprintf("no %s document in the resources index (project %s)", docType, projectUID),
	}
}
