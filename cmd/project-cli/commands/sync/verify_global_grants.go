// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands/fgaread"
)

type verifyGlobalGrantsSubcommand struct{}

func (s *verifyGlobalGrantsSubcommand) Name() string { return "verify-global-grants" }

func (s *verifyGlobalGrantsSubcommand) Help() string {
	return "compare expected global project grants from NATS with OpenFGA tuples"
}

func (s *verifyGlobalGrantsSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := commands.NewFlagSet(s.Name(), "sync "+s.Name()+" [flags]")
	concurrency := fs.Int("concurrency", 20, "max concurrent OpenFGA reads")
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
	projects, err := repo.ListAllProjectsBase(ctx)
	if err != nil {
		return fmt.Errorf("list project bases: %w", err)
	}

	report := newGlobalGrantVerificationReport()
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(*concurrency)
	for _, project := range projects {
		project := project
		group.Go(func() error {
			class := fgaread.Class(project.Slug, project.Stage)
			expected := fgaread.ExpectedTuples(project.Stage)
			actual, readErr := fgaClient.ReadProjectGlobalTuples(groupCtx, project.UID)
			mu.Lock()
			defer mu.Unlock()
			report.add(class, expected, actual, readErr)
			return nil
		})
	}
	_ = group.Wait()

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode verification report: %w", err)
	}
	if report.failed() {
		return fmt.Errorf("global grant verification failed; see aggregate report")
	}
	return nil
}

type globalGrantRelationCounts struct {
	Expected    int `json:"expected"`
	Actual      int `json:"actual"`
	Missing     int `json:"missing"`
	Unexpected  int `json:"unexpected"`
	Conditioned int `json:"conditioned"`
}

type globalGrantClassReport struct {
	Projects   int                                   `json:"projects"`
	ReadErrors int                                   `json:"read_errors"`
	Relations  map[string]*globalGrantRelationCounts `json:"relations"`
}

type globalGrantVerificationReport struct {
	Projects       int                                `json:"projects"`
	ReadErrors     int                                `json:"read_errors"`
	ReadErrorKinds map[string]int                     `json:"read_error_kinds"`
	Missing        int                                `json:"missing"`
	Unexpected     int                                `json:"unexpected"`
	Conditioned    int                                `json:"conditioned"`
	Classes        map[string]*globalGrantClassReport `json:"classes"`
}

func newGlobalGrantVerificationReport() *globalGrantVerificationReport {
	classes := make(map[string]*globalGrantClassReport, 4)
	for _, class := range []string{fgaread.ClassOrdinary, fgaread.ClassProspect, fgaread.ClassConfidential, fgaread.ClassSystemRoot} {
		classes[class] = newGlobalGrantClassReport()
	}
	return &globalGrantVerificationReport{
		ReadErrorKinds: map[string]int{},
		Classes:        classes,
	}
}

// failed reports whether the run must exit nonzero. A clean run is the gate for removing the
// legacy root grants, so every finding kind has to fail it.
func (r *globalGrantVerificationReport) failed() bool {
	return r.ReadErrors > 0 || r.Missing > 0 || r.Unexpected > 0 || r.Conditioned > 0
}

func (r *globalGrantVerificationReport) add(class string, expected, actual []fgaread.Tuple, readErr error) {
	r.Projects++
	classReport := r.Classes[class]
	if classReport == nil {
		classReport = newGlobalGrantClassReport()
		r.Classes[class] = classReport
	}
	classReport.Projects++
	for _, tuple := range expected {
		counts := relationCounts(classReport, tuple.Relation)
		counts.Expected++
	}
	if readErr != nil {
		r.ReadErrors++
		r.ReadErrorKinds[globalGrantReadErrorKind(readErr)]++
		classReport.ReadErrors++
		return
	}

	expectedSet := tupleSet(expected)
	actualSet := tupleSet(actual)
	for _, tuple := range actual {
		counts := relationCounts(classReport, tuple.Relation)
		counts.Actual++
		if tuple.Conditioned {
			counts.Conditioned++
			r.Conditioned++
		}
	}
	for key, tuple := range expectedSet {
		actualTuple, found := actualSet[key]
		if !found || actualTuple.Conditioned {
			relationCounts(classReport, tuple.Relation).Missing++
			r.Missing++
		}
	}
	for key, tuple := range actualSet {
		expectedTuple, found := expectedSet[key]
		if !found || tuple.Conditioned || expectedTuple.Conditioned {
			relationCounts(classReport, tuple.Relation).Unexpected++
			r.Unexpected++
		}
	}
}

func globalGrantReadErrorKind(err error) string {
	var httpErr fgaread.HTTPError
	switch {
	case errors.As(err, &httpErr):
		return fmt.Sprintf("http_%d", httpErr.Status)
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, fgaread.ErrRepeatedContinuationToken):
		return "repeated_continuation_token"
	case errors.Is(err, fgaread.ErrTooManyReadPages):
		return "page_limit"
	default:
		return "transport_or_decode"
	}
}

func newGlobalGrantClassReport() *globalGrantClassReport {
	report := &globalGrantClassReport{Relations: map[string]*globalGrantRelationCounts{}}
	for relation := range fgaread.ExpectedSubjects {
		report.Relations[relation] = &globalGrantRelationCounts{}
	}
	return report
}

func tupleSet(tuples []fgaread.Tuple) map[string]fgaread.Tuple {
	set := make(map[string]fgaread.Tuple, len(tuples))
	for _, tuple := range tuples {
		set[tuple.Key()] = tuple
	}
	return set
}

func relationCounts(report *globalGrantClassReport, relation string) *globalGrantRelationCounts {
	counts := report.Relations[relation]
	if counts == nil {
		counts = &globalGrantRelationCounts{}
		report.Relations[relation] = counts
	}
	return counts
}
