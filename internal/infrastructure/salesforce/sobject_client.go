// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

// Package salesforce provides a Salesforce REST API client for querying and
// mutating Salesforce objects via SOQL and the sObject REST endpoints. It wraps
// the github.com/k-capehart/go-salesforce/v3 library.
package salesforce

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	sf "github.com/k-capehart/go-salesforce/v3"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
	errs "github.com/linuxfoundation/lfx-v2-member-service/pkg/errors"
)

// sObjectCacher is the storage interface required by SObjectClient. It is
// satisfied by *nats.SObjectCache in production and by an in-memory stub in
// tests, keeping the client decoupled from the NATS infrastructure layer.
type sObjectCacher interface {
	Get(ctx context.Context, key string) (*nats.SObjectCacheEntry, error)
	Put(ctx context.Context, key string, entry *nats.SObjectCacheEntry) error
	Delete(ctx context.Context, key string) error
}

// SObjectClient wraps the Salesforce sObject REST API endpoint
// (GET /services/data/{version}/sobjects/{Type}/{Id}) and integrates HTTP
// conditional GET caching via the NATS member-service-cache KV bucket.
//
// On every fetch, the client:
//  1. Looks up the cached entry for the requested sObject UID key.
//  2. If found, adds If-None-Match (ETag) and/or If-Modified-Since headers to
//     the request so Salesforce can respond with 304 Not Modified when the
//     record is unchanged.
//  3. On 200 OK: updates the cache with the new ETag, Last-Modified, and body.
//  4. On 304 Not Modified: returns the cached body as-is without a cache write.
//  5. On cache miss: fetches without conditional headers and populates the cache.
//
// The client also supports forwarding If-Match / If-Unmodified-Since precondition
// headers from API callers through to Salesforce write endpoints via
// DoConditionalWrite.
type SObjectClient struct {
	sf    *sf.Salesforce
	cache sObjectCacher
}

// NewSObjectClient creates an SObjectClient backed by the given authenticated
// Salesforce client and NATS sObject cache.
func NewSObjectClient(sfClient *sf.Salesforce, cache *nats.SObjectCache) *SObjectClient {
	return &SObjectClient{sf: sfClient, cache: cache}
}

// FetchResult is the outcome of a conditional GET fetch via FetchSObject.
type FetchResult struct {
	// Body is the raw JSON sObject body. Populated on both 200 OK and
	// 304 Not Modified (from cache).
	Body json.RawMessage

	// ETag is the ETag from the 200 OK response, or the cached ETag on 304.
	// Empty when Salesforce did not return an ETag.
	ETag string

	// LastModified is the Last-Modified header from the 200 OK response, or the
	// cached value on 304. Empty when Salesforce did not return the header.
	LastModified string

	// NotModified is true when Salesforce returned 304 Not Modified; the Body is
	// served from the cache unchanged.
	NotModified bool
}

// FetchSObject fetches a single Salesforce sObject record by type and SFID,
// applying HTTP conditional GET caching. cacheKey must be the pre-formed
// "{sobject_type}.{uid}" key used to store this record in the NATS cache.
// fields is the comma-separated list of fields to request (e.g.
// "Id,Name,Status"). Passing an empty fields string omits the ?fields= param.
//
// The caller receives a FetchResult that always has a populated Body (either
// refreshed from Salesforce on 200 or served from the cache on 304/miss).
// A non-nil error indicates an infrastructure failure or an unexpected HTTP
// status code from Salesforce.
//
// Note: this method issues the GET directly via sf.GetHTTPClient() rather than
// sf.DoRequest(). The go-salesforce library's DoRequest discards the response
// pointer whenever the status code is outside 200–300, making it impossible to
// inspect the status code for 304 Not Modified. By using the underlying HTTP
// client directly we retain full control over response handling, context
// propagation, and future OTEL tracing.
//
// Session refresh: on a 401 Unauthorized response, FetchSObject triggers the
// library's re-authentication path via a no-op DoRequest call (which internally
// calls refreshSession and updates the stored access token), then retries the
// original request once with the renewed token.
func (c *SObjectClient) FetchSObject(ctx context.Context, sobjectType, sfid, cacheKey, fields string) (*FetchResult, error) {
	if sobjectType == "" {
		return nil, errs.NewValidation("sobjectType cannot be empty")
	}
	if sfid == "" {
		return nil, errs.NewValidation("sfid cannot be empty")
	}
	if cacheKey == "" {
		return nil, errs.NewValidation("cacheKey cannot be empty")
	}

	// Look up any existing cache entry.
	cached, err := c.cache.Get(ctx, cacheKey)
	if err != nil {
		slog.WarnContext(ctx, "sObject cache get failed; proceeding without conditional headers",
			"sobject_type", sobjectType,
			"sfid", sfid,
			"cache_key", cacheKey,
			"error", err,
		)
		// Non-fatal: proceed without conditional headers.
	}

	// Build the full sObject REST URL.
	rawURL := fmt.Sprintf("%s/services/data/%s/sobjects/%s/%s",
		c.sf.GetInstanceUrl(), c.sf.GetAPIVersion(), sobjectType, sfid)
	if fields != "" {
		rawURL += "?fields=" + fields
	}

	resp, err := c.doGet(ctx, rawURL, cached)
	if err != nil {
		return nil, fmt.Errorf("salesforce sObject GET %s/%s: %w", sobjectType, sfid, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// On 401 Unauthorized: delegate to the library for session refresh (it knows
	// the grant type and stored credentials), then retry once.
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close() //nolint:errcheck
		if refreshErr := c.refreshSession(); refreshErr != nil {
			return nil, fmt.Errorf("salesforce sObject GET %s/%s: session refresh: %w",
				sobjectType, sfid, refreshErr)
		}
		resp, err = c.doGet(ctx, rawURL, cached)
		if err != nil {
			return nil, fmt.Errorf("salesforce sObject GET %s/%s (retry): %w", sobjectType, sfid, err)
		}
		defer resp.Body.Close() //nolint:errcheck
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return c.handle200(ctx, resp, cacheKey)
	case http.StatusNotModified:
		return c.handle304(ctx, cached, sobjectType, sfid, cacheKey)
	default:
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("salesforce sObject GET %s/%s: unexpected status %d: %s",
			sobjectType, sfid, resp.StatusCode, body)
	}
}

// doGet issues a single authenticated GET to rawURL, attaching conditional
// headers from cached if present. It returns the raw response; the caller is
// responsible for closing the body.
func (c *SObjectClient) doGet(ctx context.Context, rawURL string, cached *nats.SObjectCacheEntry) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.sf.GetAccessToken())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	if cached != nil {
		if cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		if cached.LastModified != "" {
			req.Header.Set("If-Modified-Since", cached.LastModified)
		}
	}

	return c.sf.GetHTTPClient().Do(req)
}

// refreshSession triggers the go-salesforce library's internal session-refresh
// path by issuing a DoRequest call that the library will retry with refreshed
// credentials on INVALID_SESSION_ID. The /limits endpoint is used because it is
// lightweight, read-only, and is the same endpoint called during sf.Init.
func (c *SObjectClient) refreshSession() error {
	uri := fmt.Sprintf("/services/data/%s/limits", c.sf.GetAPIVersion())
	_, err := c.sf.DoRequest(http.MethodGet, uri, nil)
	if err != nil {
		return err
	}
	return nil
}

// handle200 processes a 200 OK response: reads the body, extracts ETag and
// Last-Modified, updates the cache, and returns a FetchResult.
func (c *SObjectClient) handle200(ctx context.Context, resp *http.Response, cacheKey string) (*FetchResult, error) {
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading sObject response body: %w", err)
	}

	etag := resp.Header.Get("ETag")
	lastModified := resp.Header.Get("Last-Modified")

	entry := &nats.SObjectCacheEntry{
		ETag:         etag,
		LastModified: lastModified,
		Body:         json.RawMessage(rawBody),
	}

	if putErr := c.cache.Put(ctx, cacheKey, entry); putErr != nil {
		// Non-fatal: log and continue; the caller still gets a valid response.
		slog.WarnContext(ctx, "failed to update sObject cache after 200 OK",
			"cache_key", cacheKey,
			"error", putErr,
		)
	}

	return &FetchResult{
		Body:         json.RawMessage(rawBody),
		ETag:         etag,
		LastModified: lastModified,
		NotModified:  false,
	}, nil
}

// handle304 processes a 304 Not Modified response: serves the body from the
// existing cache entry. If no cached entry is available (should not normally
// happen — Salesforce should only return 304 when we sent conditional headers,
// which requires a cached entry), an error is returned.
func (c *SObjectClient) handle304(ctx context.Context, cached *nats.SObjectCacheEntry, sobjectType, sfid, cacheKey string) (*FetchResult, error) {
	if cached == nil {
		// This is a protocol violation: Salesforce returned 304 without us sending
		// conditional headers. Treat it as an error.
		return nil, fmt.Errorf("salesforce sObject GET %s/%s: received 304 Not Modified but no cached entry exists for key %q",
			sobjectType, sfid, cacheKey)
	}

	slog.DebugContext(ctx, "sObject cache hit (304 Not Modified)",
		"sobject_type", sobjectType,
		"sfid", sfid,
		"cache_key", cacheKey,
	)

	return &FetchResult{
		Body:         cached.Body,
		ETag:         cached.ETag,
		LastModified: cached.LastModified,
		NotModified:  true,
	}, nil
}

// DoConditionalWrite performs a write (POST/PATCH/DELETE) against a Salesforce
// sObject endpoint, forwarding any If-Match or If-Unmodified-Since precondition
// headers supplied by the API caller. This enables optimistic concurrency
// control: Salesforce returns 412 Precondition Failed when the record has been
// modified since the client last read it.
//
// ifMatch is the value to send as the If-Match request header (typically the
// ETag of the last known version). ifUnmodifiedSince is the value to send as
// If-Unmodified-Since. Both are optional; pass an empty string to omit.
//
// The raw *http.Response is returned; the caller is responsible for closing the
// response body. A non-nil error indicates a transport-level failure.
func (c *SObjectClient) DoConditionalWrite(
	ctx context.Context,
	method, uri string,
	body []byte,
	ifMatch, ifUnmodifiedSince string,
) (*http.Response, error) {
	var opts []sf.RequestOption
	if ifMatch != "" {
		opts = append(opts, sf.WithHeader("If-Match", ifMatch))
	}
	if ifUnmodifiedSince != "" {
		opts = append(opts, sf.WithHeader("If-Unmodified-Since", ifUnmodifiedSince))
	}

	resp, err := c.sf.DoRequest(method, uri, body, opts...)
	if err != nil {
		return nil, fmt.Errorf("salesforce conditional write %s %s: %w", method, uri, err)
	}
	return resp, nil
}

// InvalidateCache removes the cached sObject entry for the given cache key.
// Call this after a successful write so the next read fetches fresh data from
// Salesforce. Returns nil if the key does not exist.
func (c *SObjectClient) InvalidateCache(ctx context.Context, cacheKey string) error {
	return c.cache.Delete(ctx, cacheKey)
}
