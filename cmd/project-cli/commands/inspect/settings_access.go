// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package inspect

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	fgaconstants "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/constants"
	fgatypes "github.com/linuxfoundation/lfx-v2-fga-sync/pkg/types"
	"golang.org/x/sync/errgroup"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/internal/fgaread"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/domain/models"
	"github.com/linuxfoundation/lfx-v2-project-service/internal/service"
)

// Finding kinds reported by inspect settings-access.
const (
	kindMissingSettings    = "missing_settings"
	kindAccessMissing      = "access_missing"
	kindAccessUnexpected   = "access_unexpected"
	kindAccessReadError    = "access_read_error"
	kindAccessConditioned  = "access_conditioned"
	defaultAccessReadLimit = 20
)

// accessRepo is the slice of the repository the access check reads.
type accessRepo interface {
	ListAllProjectsBase(ctx context.Context) ([]*models.ProjectBase, error)
	ListAllProjectsSettings(ctx context.Context) ([]*models.ProjectSettings, error)
}

// tupleReader reads the tuples of one project whose relation keep accepts.
type tupleReader interface {
	ReadProjectTuples(ctx context.Context, projectUID string, keep func(relation string) bool) ([]fgaread.Tuple, error)
}

type settingsAccessSubcommand struct{}

func (s *settingsAccessSubcommand) Name() string { return "settings-access" }

func (s *settingsAccessSubcommand) Help() string {
	return "compare each project's settings roster and parent with its direct OpenFGA tuples"
}

func (s *settingsAccessSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "inspect settings-access [flags]")
	concurrency := fs.Int("concurrency", defaultAccessReadLimit, "max concurrent OpenFGA reads")
	if help, err := commands.ParseFlags(fs, rc.Args); help || err != nil {
		return err
	}
	if err := commands.RejectExtraArgs(fs); err != nil {
		return err
	}
	if *concurrency < 1 || *concurrency > 100 {
		return fmt.Errorf("concurrency must be between 1 and 100")
	}

	fgaClient, err := fgaread.NewClientFromEnv()
	if err != nil {
		return err
	}
	if err := fgaClient.CheckStore(ctx); err != nil {
		return err
	}
	natsConn, repo, err := commands.ConnectRepository(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()

	return runSettingsAccess(ctx, repo, fgaClient, *concurrency, os.Stdout)
}

type settingsAccessReport struct {
	Projects int            `json:"projects"`
	Counts   map[string]int `json:"counts"`
	Findings []finding      `json:"findings"`
}

// expectedAccess is what the access message builder would write for a project,
// restricted to the relations that come from project settings and the parent.
type expectedAccess struct {
	relations map[string]struct{} // the relations the check covers
	tuples    map[string]struct{} // "<relation>\x00<user>"
}

// buildExpectedAccess asks the production message builder what it would
// publish, so the check follows the real roster rules (email-only users have no
// username and get no tuple) instead of restating them.
func buildExpectedAccess(base *models.ProjectBase, settings *models.ProjectSettings) expectedAccess {
	msg := service.NewProjectProjection(base, settings).ToFGAMessageKeepingDirectUsers()
	data, _ := msg.Data.(fgatypes.GenericAccessData)

	expected := expectedAccess{
		relations: map[string]struct{}{fgaconstants.RelationParent: {}},
		tuples:    map[string]struct{}{},
	}
	for _, relation := range data.ExcludeRelations {
		expected.relations[relation] = struct{}{}
	}
	for relation, users := range data.Relations {
		for _, user := range users {
			expected.tuples[accessKey(relation, fgaconstants.ObjectTypeUser+user)] = struct{}{}
		}
	}
	for _, parent := range data.References[fgaconstants.RelationParent] {
		expected.tuples[accessKey(fgaconstants.RelationParent, parent)] = struct{}{}
	}
	return expected
}

func accessKey(relation, user string) string { return relation + "\x00" + user }

// runSettingsAccess compares, for every project, the direct user and parent
// tuples the settings call for with the ones OpenFGA holds. Findings carry
// counts per relation and never name a user.
func runSettingsAccess(ctx context.Context, repo accessRepo, reader tupleReader, concurrency int, out io.Writer) error {
	bases, err := repo.ListAllProjectsBase(ctx)
	if err != nil {
		return fmt.Errorf("list project bases: %w", err)
	}
	allSettings, err := repo.ListAllProjectsSettings(ctx)
	if err != nil {
		return fmt.Errorf("list project settings: %w", err)
	}
	settingsByUID := make(map[string]*models.ProjectSettings, len(allSettings))
	for _, settings := range allSettings {
		settingsByUID[settings.UID] = settings
	}

	report := &settingsAccessReport{Projects: len(bases), Counts: map[string]int{}, Findings: []finding{}}
	var mu sync.Mutex
	add := func(f finding) {
		mu.Lock()
		defer mu.Unlock()
		report.Findings = append(report.Findings, f)
		report.Counts[f.Kind]++
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(concurrency)
	for _, base := range bases {
		base := base
		settings, ok := settingsByUID[base.UID]
		if !ok {
			add(finding{Kind: kindMissingSettings, Slug: base.Slug, UID: base.UID, Detail: "no settings record, so the roster cannot be checked"})
			continue
		}
		group.Go(func() error {
			for _, f := range checkProjectAccess(groupCtx, reader, base, settings) {
				add(f)
			}
			return nil
		})
	}
	_ = group.Wait()
	sortFindings(report.Findings)

	if err := writeReport(out, report); err != nil {
		return err
	}
	return problemsError("inspect settings-access", len(report.Findings))
}

func checkProjectAccess(ctx context.Context, reader tupleReader, base *models.ProjectBase, settings *models.ProjectSettings) []finding {
	expected := buildExpectedAccess(base, settings)
	actual, err := reader.ReadProjectTuples(ctx, base.UID, func(relation string) bool {
		_, covered := expected.relations[relation]
		return covered
	})
	if err != nil {
		return []finding{{Kind: kindAccessReadError, Slug: base.Slug, UID: base.UID, Detail: err.Error()}}
	}

	missing := map[string]int{}
	unexpected := map[string]int{}
	conditioned := map[string]int{}
	seen := map[string]struct{}{}
	for _, tuple := range actual {
		key := accessKey(tuple.Relation, tuple.User)
		seen[key] = struct{}{}
		if _, ok := expected.tuples[key]; !ok {
			unexpected[tuple.Relation]++
		}
		if tuple.Conditioned {
			conditioned[tuple.Relation]++
		}
	}
	for key := range expected.tuples {
		if _, ok := seen[key]; !ok {
			missing[strings.SplitN(key, "\x00", 2)[0]]++
		}
	}

	var findings []finding
	for kind, counts := range map[string]map[string]int{
		kindAccessMissing:     missing,
		kindAccessUnexpected:  unexpected,
		kindAccessConditioned: conditioned,
	} {
		if len(counts) > 0 {
			findings = append(findings, finding{Kind: kind, Slug: base.Slug, UID: base.UID, Detail: describeCounts(counts)})
		}
	}
	return findings
}

// describeCounts renders per-relation counts in a stable order.
func describeCounts(counts map[string]int) string {
	relations := make([]string, 0, len(counts))
	for relation := range counts {
		relations = append(relations, relation)
	}
	sort.Strings(relations)
	parts := make([]string, 0, len(relations))
	for _, relation := range relations {
		parts = append(parts, fmt.Sprintf("%s=%d", relation, counts[relation]))
	}
	return strings.Join(parts, ", ")
}
