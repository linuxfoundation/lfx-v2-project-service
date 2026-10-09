// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/linuxfoundation/lfx-v2-project-service/cmd/project-cli/commands"
	natsinfra "github.com/linuxfoundation/lfx-v2-project-service/internal/infrastructure/nats"
)

const (
	classOrdinary     = "ordinary"
	classProspect     = "prospect"
	classConfidential = "confidential"
	classSystemRoot   = "system_root"
	maxFGAReadPages   = 1000
)

var ulidPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
var errRepeatedContinuationToken = errors.New("OpenFGA repeated a continuation token")
var errTooManyReadPages = errors.New("OpenFGA read exceeded the page limit")

// This verification contract is deliberately independent of the production
// grant builder. A regression in that builder must not alter both sides of the
// comparison in the same process.
var expectedGlobalSubjects = map[string][]string{
	"global_owner":         {"team:formation#member", "team:product-support#member"},
	"global_writer":        {"team:global-project-writers#member"},
	"global_auditor":       {"team:lf-staff#member", "team:global-project-auditors#member"},
	"global_marketing_ops": {"team:marketing-ops#member"},
}

type verifyGlobalGrantsSubcommand struct{}

func (s *verifyGlobalGrantsSubcommand) Name() string { return "verify-global-grants" }

func (s *verifyGlobalGrantsSubcommand) Help() string {
	return "compare expected global project grants from NATS with OpenFGA tuples"
}

func (s *verifyGlobalGrantsSubcommand) Run(ctx context.Context, rc commands.RunContext) error {
	fs := flag.NewFlagSet(s.Name(), flag.ContinueOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "usage: project-cli sync %s [flags]\n\nflags:\n", s.Name())
		fs.PrintDefaults()
	}
	concurrency := fs.Int("concurrency", 20, "max concurrent OpenFGA reads")
	if err := fs.Parse(rc.Args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *concurrency < 1 || *concurrency > 100 {
		return fmt.Errorf("concurrency must be between 1 and 100")
	}

	fgaClient, err := newGlobalGrantFGAClientFromEnv()
	if err != nil {
		return err
	}
	if err := fgaClient.checkStore(ctx); err != nil {
		return err
	}
	natsConn, js, err := natsinfra.Connect(ctx, rc.NATSConfig)
	if err != nil {
		return err
	}
	defer natsConn.Close()
	repo, err := natsinfra.OpenRepository(ctx, js)
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}
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
			class := globalGrantClass(project.Slug, project.Stage)
			expected := expectedGlobalGrantTuples(project.Stage)
			actual, readErr := fgaClient.readProjectGlobalTuples(groupCtx, project.UID)
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

type globalGrantTuple struct {
	User        string
	Relation    string
	Conditioned bool
}

func (t globalGrantTuple) key() string {
	return t.User + "\x00" + t.Relation
}

type globalGrantFGAClient struct {
	baseURL string
	storeID string
	http    *http.Client
}

func newGlobalGrantFGAClientFromEnv() (*globalGrantFGAClient, error) {
	rawURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENFGA_API_URL")), "/")
	storeID := strings.TrimSpace(os.Getenv("OPENFGA_STORE_ID"))
	if rawURL == "" || storeID == "" {
		return nil, fmt.Errorf("OPENFGA_API_URL and OPENFGA_STORE_ID are required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("OPENFGA_API_URL must be an absolute HTTP(S) URL")
	}
	if !ulidPattern.MatchString(storeID) {
		return nil, fmt.Errorf("OPENFGA_STORE_ID must be a ULID")
	}
	return &globalGrantFGAClient{
		baseURL: rawURL,
		storeID: storeID,
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// checkStore fails fast on an absent store. OpenFGA answers a read against an
// unknown store ID with HTTP 200 and no tuples, which would otherwise surface as
// every expected grant missing rather than as a configuration error.
func (c *globalGrantFGAClient) checkStore(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/stores/%s", c.baseURL, c.storeID), nil)
	if err != nil {
		return fmt.Errorf("create OpenFGA store request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error embeds the request URL, which carries the store ID.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("OpenFGA store check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("OPENFGA_STORE_ID does not name an existing OpenFGA store")
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		return fmt.Errorf("OpenFGA store check returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *globalGrantFGAClient) readProjectGlobalTuples(ctx context.Context, projectUID string) ([]globalGrantTuple, error) {
	var tuples []globalGrantTuple
	token := ""
	seenTokens := map[string]struct{}{}
	for pageNumber := 0; pageNumber < maxFGAReadPages; pageNumber++ {
		requestBody := map[string]any{
			"tuple_key":   map[string]string{"object": "project:" + projectUID},
			"consistency": "HIGHER_CONSISTENCY",
			"page_size":   100,
		}
		if token != "" {
			requestBody["continuation_token"] = token
		}
		raw, err := json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("marshal OpenFGA request: %w", err)
		}
		endpoint := fmt.Sprintf("%s/stores/%s/read", c.baseURL, c.storeID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("create OpenFGA request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			// *url.Error embeds the request URL, which carries the store ID.
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				err = urlErr.Err
			}
			return nil, fmt.Errorf("OpenFGA read failed: %w", err)
		}
		page, err := decodeGlobalGrantPage(resp)
		if err != nil {
			return nil, err
		}
		for _, tuple := range page.Tuples {
			if strings.HasPrefix(tuple.Key.Relation, "global_") {
				tuples = append(tuples, globalGrantTuple{
					User:        tuple.Key.User,
					Relation:    tuple.Key.Relation,
					Conditioned: hasTupleCondition(tuple.Key.Condition),
				})
			}
		}
		token = page.ContinuationToken
		if token == "" {
			return tuples, nil
		}
		if _, exists := seenTokens[token]; exists {
			return nil, errRepeatedContinuationToken
		}
		seenTokens[token] = struct{}{}
	}
	return nil, errTooManyReadPages
}

type globalGrantReadPage struct {
	Tuples []struct {
		Key struct {
			User      string          `json:"user"`
			Relation  string          `json:"relation"`
			Condition json.RawMessage `json:"condition"`
		} `json:"key"`
	} `json:"tuples"`
	ContinuationToken string `json:"continuation_token"`
}

func decodeGlobalGrantPage(resp *http.Response) (*globalGrantReadPage, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, globalGrantHTTPError{status: resp.StatusCode}
	}
	var page globalGrantReadPage
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	if err := decoder.Decode(&page); err != nil {
		return nil, fmt.Errorf("decode OpenFGA response: %w", err)
	}
	return &page, nil
}

type globalGrantHTTPError struct {
	status int
}

func (e globalGrantHTTPError) Error() string {
	return fmt.Sprintf("OpenFGA read returned HTTP %d", e.status)
}

func hasTupleCondition(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}"
}

func globalGrantClass(slug, stage string) string {
	switch {
	case slug == rootProjectSlug:
		return classSystemRoot
	case stage == "Prospect":
		return classProspect
	case stage == "Formation - Confidential":
		return classConfidential
	default:
		return classOrdinary
	}
}

func expectedGlobalGrantTuples(stage string) []globalGrantTuple {
	relations := []string{"global_owner"}
	if stage != "Prospect" && stage != "Formation - Confidential" {
		relations = append(relations, "global_writer", "global_auditor", "global_marketing_ops")
	}
	var tuples []globalGrantTuple
	for _, relation := range relations {
		for _, subject := range expectedGlobalSubjects[relation] {
			tuples = append(tuples, globalGrantTuple{User: subject, Relation: relation})
		}
	}
	return tuples
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
	for _, class := range []string{classOrdinary, classProspect, classConfidential, classSystemRoot} {
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

func (r *globalGrantVerificationReport) add(class string, expected, actual []globalGrantTuple, readErr error) {
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
	var httpErr globalGrantHTTPError
	switch {
	case errors.As(err, &httpErr):
		return fmt.Sprintf("http_%d", httpErr.status)
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, errRepeatedContinuationToken):
		return "repeated_continuation_token"
	case errors.Is(err, errTooManyReadPages):
		return "page_limit"
	default:
		return "transport_or_decode"
	}
}

func newGlobalGrantClassReport() *globalGrantClassReport {
	report := &globalGrantClassReport{Relations: map[string]*globalGrantRelationCounts{}}
	for relation := range expectedGlobalSubjects {
		report.Relations[relation] = &globalGrantRelationCounts{}
	}
	return report
}

func tupleSet(tuples []globalGrantTuple) map[string]globalGrantTuple {
	set := make(map[string]globalGrantTuple, len(tuples))
	for _, tuple := range tuples {
		set[tuple.key()] = tuple
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
