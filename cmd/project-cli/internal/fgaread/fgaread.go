// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package fgaread reads the global project grant tuples from OpenFGA and
// holds the contract they are checked against. It is shared by the sync and
// inspect command groups.
package fgaread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/linuxfoundation/lfx-v2-project-service/pkg/constants"
)

// Project classes, which decide the global grants a project is expected to carry.
const (
	ClassOrdinary     = "ordinary"
	ClassProspect     = "prospect"
	ClassConfidential = "confidential"
	ClassSystemRoot   = "system_root"
	maxReadPages      = 1000
)

var ulidPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

// ErrRepeatedContinuationToken reports a read that OpenFGA could not page through.
var ErrRepeatedContinuationToken = errors.New("OpenFGA repeated a continuation token")

// ErrTooManyReadPages reports a read that exceeded the page limit.
var ErrTooManyReadPages = errors.New("OpenFGA read exceeded the page limit")

// This verification contract is deliberately independent of the production
// grant builder. A regression in that builder must not alter both sides of the
// comparison in the same process.
var ExpectedSubjects = map[string][]string{
	"global_owner":         {"team:formation#member", "team:product-support#member"},
	"global_writer":        {"team:global-project-writers#member"},
	"global_auditor":       {"team:lf-staff#member", "team:global-project-auditors#member"},
	"global_marketing_ops": {"team:marketing-ops#member"},
}

// Tuple is one global_* tuple on a project object.
type Tuple struct {
	User        string
	Relation    string
	Conditioned bool
}

// Key identifies the tuple by user and relation.
func (t Tuple) Key() string {
	return t.User + "\x00" + t.Relation
}

// Client reads tuples from one OpenFGA store.
type Client struct {
	baseURL string
	storeID string
	http    *http.Client
}

// NewClientFromEnv builds a Client from OPENFGA_API_URL and OPENFGA_STORE_ID.
func NewClientFromEnv() (*Client, error) {
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
	return &Client{
		baseURL: rawURL,
		storeID: storeID,
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// CheckStore fails fast on an absent store. OpenFGA answers a read against an
// unknown store ID with HTTP 200 and no tuples, which would otherwise surface as
// every expected grant missing rather than as a configuration error.
func (c *Client) CheckStore(ctx context.Context) error {
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

// ReadProjectGlobalTuples returns the global_* tuples on a project object.
func (c *Client) ReadProjectGlobalTuples(ctx context.Context, projectUID string) ([]Tuple, error) {
	return c.ReadProjectTuples(ctx, projectUID, func(relation string) bool {
		return strings.HasPrefix(relation, "global_")
	})
}

// ReadProjectTuples returns the tuples on a project object whose relation keep accepts.
func (c *Client) ReadProjectTuples(ctx context.Context, projectUID string, keep func(relation string) bool) ([]Tuple, error) {
	var tuples []Tuple
	token := ""
	seenTokens := map[string]struct{}{}
	for pageNumber := 0; pageNumber < maxReadPages; pageNumber++ {
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
		page, err := decodePage(resp)
		if err != nil {
			return nil, err
		}
		for _, tuple := range page.Tuples {
			if keep(tuple.Key.Relation) {
				tuples = append(tuples, Tuple{
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
			return nil, ErrRepeatedContinuationToken
		}
		seenTokens[token] = struct{}{}
	}
	return nil, ErrTooManyReadPages
}

type readPage struct {
	Tuples []struct {
		Key struct {
			User      string          `json:"user"`
			Relation  string          `json:"relation"`
			Condition json.RawMessage `json:"condition"`
		} `json:"key"`
	} `json:"tuples"`
	ContinuationToken string `json:"continuation_token"`
}

func decodePage(resp *http.Response) (*readPage, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, HTTPError{Status: resp.StatusCode}
	}
	var page readPage
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	if err := decoder.Decode(&page); err != nil {
		return nil, fmt.Errorf("decode OpenFGA response: %w", err)
	}
	return &page, nil
}

// HTTPError is a non-2xx OpenFGA response.
type HTTPError struct {
	Status int
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("OpenFGA read returned HTTP %d", e.Status)
}

func hasTupleCondition(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}"
}

// Class names the grant class of a project from its slug and stage.
func Class(slug, stage string) string {
	switch {
	case slug == constants.RootProjectSlug:
		return ClassSystemRoot
	case stage == "Prospect":
		return ClassProspect
	case stage == "Formation - Confidential":
		return ClassConfidential
	default:
		return ClassOrdinary
	}
}

// ExpectedTuples lists the global grants a project at the given stage must carry.
func ExpectedTuples(stage string) []Tuple {
	relations := []string{"global_owner"}
	if stage != "Prospect" && stage != "Formation - Confidential" {
		relations = append(relations, "global_writer", "global_auditor", "global_marketing_ops")
	}
	var tuples []Tuple
	for _, relation := range relations {
		for _, subject := range ExpectedSubjects[relation] {
			tuples = append(tuples, Tuple{User: subject, Relation: relation})
		}
	}
	return tuples
}
