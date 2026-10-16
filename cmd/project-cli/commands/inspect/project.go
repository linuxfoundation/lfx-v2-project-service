// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	opensearchgo "github.com/opensearch-project/opensearch-go/v2"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/fgaread"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/osdiff"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	osinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/opensearch"
	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// projectRepo is the slice of the repository inspect project reads.
type projectRepo interface {
	GetProjectBase(ctx context.Context, projectUID string) (*models.ProjectBase, error)
	GetProjectSettings(ctx context.Context, projectUID string) (*models.ProjectSettings, error)
	GetProjectUIDFromSlug(ctx context.Context, projectSlug string) (string, error)
	ListAllLinks(ctx context.Context, projectUID string) ([]*models.ProjectLink, error)
	ListFolders(ctx context.Context, projectUID string) ([]*models.ProjectFolder, error)
	ListAllDocuments(ctx context.Context, projectUID string) ([]*models.ProjectDocument, error)
}

// grantReader reads the global grant tuples of one project.
type grantReader interface {
	ReadProjectGlobalTuples(ctx context.Context, projectUID string) ([]fgaread.Tuple, error)
}

type projectSubcommand struct{}

func (s *projectSubcommand) Name() string { return "project" }

func (s *projectSubcommand) Help() string {
	return "show a project's KV records, slug index entry, OpenSearch documents and global OpenFGA grants"
}

func (s *projectSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect project [flags] <uid|slug>")
	skipOpenSearch := fs.Bool("skip-opensearch", false, "do not check the OpenSearch documents")
	skipFGA := fs.Bool("skip-fga", false, "do not check the OpenFGA global grants")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("expected exactly one project UID or slug, got %d argument(s)", fs.NArg())
	}
	target := strings.TrimSpace(fs.Arg(0))
	if target == "" {
		return fmt.Errorf("project UID or slug must not be empty")
	}

	inspector := &projectInspector{}
	var skipped []string

	if *skipFGA {
		skipped = append(skipped, "openfga: --skip-fga")
	} else if os.Getenv("OPENFGA_API_URL") == "" && os.Getenv("OPENFGA_STORE_ID") == "" {
		skipped = append(skipped, "openfga: OPENFGA_API_URL and OPENFGA_STORE_ID are not set")
	} else {
		fgaClient, err := fgaread.NewClientFromEnv()
		if err != nil {
			return err
		}
		if err := fgaClient.CheckStore(ctx); err != nil {
			return err
		}
		inspector.grants = fgaClient
	}

	if *skipOpenSearch {
		skipped = append(skipped, "opensearch: --skip-opensearch")
	} else {
		osClient, err := osinfra.NewClient(ctx, rc.OpenSearchConfig)
		if err != nil {
			return err
		}
		inspector.openSearch = osClient
	}

	natsConn, repo, err := commands.ConnectRepository(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()
	inspector.repo = repo

	return inspector.run(ctx, target, skipped, os.Stdout)
}

type projectInspector struct {
	repo       projectRepo
	openSearch *opensearchgo.Client
	grants     grantReader
}

type slugIndexCheck struct {
	Present  bool   `json:"present"`
	PointsTo string `json:"points_to,omitempty"`
}

type openSearchCheck struct {
	ProjectDocument  bool `json:"project_document"`
	SettingsDocument bool `json:"settings_document"`
}

type grantCheck struct {
	Class       string   `json:"class"`
	Expected    int      `json:"expected"`
	Actual      int      `json:"actual"`
	Missing     []string `json:"missing,omitempty"`
	Unexpected  []string `json:"unexpected,omitempty"`
	Conditioned []string `json:"conditioned,omitempty"`
}

type childCounts struct {
	Links     int `json:"links"`
	Folders   int `json:"folders"`
	Documents int `json:"documents"`
}

type projectReport struct {
	UID             string           `json:"uid"`
	Slug            string           `json:"slug"`
	Name            string           `json:"name"`
	Stage           string           `json:"stage"`
	SettingsPresent bool             `json:"settings_present"`
	SlugIndex       slugIndexCheck   `json:"slug_index"`
	OpenSearch      *openSearchCheck `json:"opensearch,omitempty"`
	Grants          *grantCheck      `json:"global_grants,omitempty"`
	Children        childCounts      `json:"children"`
	Skipped         []string         `json:"skipped,omitempty"`
	Problems        []string         `json:"problems"`
}

// resolveProjectUID accepts a project UID or a slug. A UUID is taken as a UID;
// anything else is looked up in the slug index.
func (p *projectInspector) resolveProjectUID(ctx context.Context, target string) (string, error) {
	if _, err := uuid.Parse(target); err == nil {
		return target, nil
	}
	uid, err := p.repo.GetProjectUIDFromSlug(ctx, target)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			return "", fmt.Errorf("no slug index entry for %q", target)
		}
		return "", fmt.Errorf("resolve slug %q: %w", target, err)
	}
	return uid, nil
}

func (p *projectInspector) run(ctx context.Context, target string, skipped []string, out io.Writer) error {
	uid, err := p.resolveProjectUID(ctx, target)
	if err != nil {
		return err
	}
	base, err := p.repo.GetProjectBase(ctx, uid)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			return fmt.Errorf("project %q not found", uid)
		}
		return fmt.Errorf("get project base %q: %w", uid, err)
	}

	report := &projectReport{
		UID:     base.UID,
		Slug:    base.Slug,
		Name:    base.Name,
		Stage:   base.Stage,
		Skipped: skipped,
	}
	problem := func(format string, args ...any) {
		report.Problems = append(report.Problems, fmt.Sprintf(format, args...))
	}

	settings, err := p.repo.GetProjectSettings(ctx, uid)
	switch {
	case err == nil && settings != nil:
		report.SettingsPresent = true
	case err == nil || errors.Is(err, domain.ErrProjectNotFound):
		problem("project settings record is missing")
	default:
		return fmt.Errorf("get project settings %q: %w", uid, err)
	}

	indexedUID, err := p.repo.GetProjectUIDFromSlug(ctx, base.Slug)
	switch {
	case err == nil:
		report.SlugIndex = slugIndexCheck{Present: true, PointsTo: indexedUID}
		if indexedUID != uid {
			problem("slug index entry %q points at %q, not this project", base.Slug, indexedUID)
		}
	case errors.Is(err, domain.ErrProjectNotFound):
		problem("slug index entry %q is missing", base.Slug)
	default:
		return fmt.Errorf("read slug index %q: %w", base.Slug, err)
	}

	if err := p.checkOpenSearch(ctx, base, report, problem); err != nil {
		return err
	}
	if err := p.checkGrants(ctx, base, report, problem); err != nil {
		return err
	}
	if err := p.countChildren(ctx, uid, report); err != nil {
		return err
	}

	if report.Problems == nil {
		report.Problems = []string{}
	}
	if err := writeReport(out, report); err != nil {
		return err
	}
	return problemsError("inspect project", len(report.Problems))
}

func (p *projectInspector) checkOpenSearch(ctx context.Context, base *models.ProjectBase, report *projectReport, problem func(string, ...any)) error {
	if p.openSearch == nil {
		return nil
	}
	if base.Slug == constants.RootProjectSlug {
		report.Skipped = append(report.Skipped, "opensearch: the hidden ROOT project is never indexed")
		return nil
	}
	missing, err := osdiff.Diff(ctx, p.openSearch, []*models.ProjectBase{base})
	if err != nil {
		return fmt.Errorf("diff opensearch: %w", err)
	}
	m := missing[base.UID]
	report.OpenSearch = &openSearchCheck{ProjectDocument: !m.Project, SettingsDocument: !m.Settings}
	if m.Project {
		problem("OpenSearch project document is missing")
	}
	if m.Settings {
		problem("OpenSearch project_settings document is missing")
	}
	return nil
}

func (p *projectInspector) checkGrants(ctx context.Context, base *models.ProjectBase, report *projectReport, problem func(string, ...any)) error {
	if p.grants == nil {
		return nil
	}
	expected := fgaread.ExpectedTuples(base.Stage)
	actual, err := p.grants.ReadProjectGlobalTuples(ctx, base.UID)
	if err != nil {
		return fmt.Errorf("read OpenFGA grants: %w", err)
	}

	check := &grantCheck{
		Class:    fgaread.Class(base.Slug, base.Stage),
		Expected: len(expected),
		Actual:   len(actual),
	}
	actualByKey := make(map[string]fgaread.Tuple, len(actual))
	for _, tuple := range actual {
		actualByKey[tuple.Key()] = tuple
	}
	expectedByKey := make(map[string]struct{}, len(expected))
	for _, tuple := range expected {
		expectedByKey[tuple.Key()] = struct{}{}
		if _, ok := actualByKey[tuple.Key()]; !ok {
			check.Missing = append(check.Missing, describeTuple(tuple))
		}
	}
	for key, tuple := range actualByKey {
		if _, ok := expectedByKey[key]; !ok {
			check.Unexpected = append(check.Unexpected, describeTuple(tuple))
		}
		if tuple.Conditioned {
			check.Conditioned = append(check.Conditioned, describeTuple(tuple))
		}
	}
	sort.Strings(check.Missing)
	sort.Strings(check.Unexpected)
	sort.Strings(check.Conditioned)
	report.Grants = check

	if n := len(check.Missing); n > 0 {
		problem("%d expected global grant(s) are missing", n)
	}
	if n := len(check.Unexpected); n > 0 {
		problem("%d unexpected global grant(s) are present", n)
	}
	if n := len(check.Conditioned); n > 0 {
		problem("%d global grant(s) carry a condition", n)
	}
	return nil
}

func describeTuple(t fgaread.Tuple) string { return t.Relation + " " + t.User }

func (p *projectInspector) countChildren(ctx context.Context, uid string, report *projectReport) error {
	// Links are counted from the records, not the per-project lookup keys, so a link
	// whose lookup key is missing still shows up here (inspect children reports it).
	allLinks, err := p.repo.ListAllLinks(ctx, "")
	if err != nil {
		return fmt.Errorf("list links: %w", err)
	}
	links := 0
	for _, link := range allLinks {
		if link.ProjectUID == uid {
			links++
		}
	}
	folders, err := p.repo.ListFolders(ctx, uid)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	documents, err := p.repo.ListAllDocuments(ctx, uid)
	if err != nil {
		return fmt.Errorf("list documents: %w", err)
	}
	report.Children = childCounts{Links: links, Folders: len(folders), Documents: len(documents)}
	return nil
}
