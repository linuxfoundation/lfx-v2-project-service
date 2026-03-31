// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package salesforce

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	sf "github.com/k-capehart/go-salesforce/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/linuxfoundation/lfx-v2-member-service/internal/infrastructure/nats"
)

// ─── test helpers ─────────────────────────────────────────────────────────────

// fakeSalesforce initialises a *sf.Salesforce using the access-token auth flow
// (no network call) and injects the given RoundTripper so tests can control HTTP
// responses without a real Salesforce instance.
//
// The go-salesforce library issues a GET /limits request during Init to validate
// the access token. The transport must handle that request (and any subsequent
// sObject requests) by routing on URL path.
func fakeSalesforce(t *testing.T, rt http.RoundTripper) *sf.Salesforce {
	t.Helper()
	client, err := sf.Init(
		sf.Creds{
			Domain:      "https://test.salesforce.com",
			AccessToken: "fake-token-for-tests",
		},
		sf.WithRoundTripper(rt),
	)
	require.NoError(t, err, "sf.Init should succeed with access-token flow")
	return client
}

// routingTransport is an http.RoundTripper that dispatches requests to
// different handlers based on a URL path prefix. The fallback handler is used
// when no prefix matches. This lets tests control the /limits initialisation
// call (used by sf.Init) separately from the sObject REST call under test.
type routingTransport struct {
	mu       sync.Mutex
	routes   []routingRule
	requests []*http.Request
}

type routingRule struct {
	pathContains string
	response     *http.Response
}

// route adds a URL-path rule. The first matching rule wins.
func (rt *routingTransport) route(pathContains string, resp *http.Response) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.routes = append(rt.routes, routingRule{pathContains: pathContains, response: resp})
}

func (rt *routingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.requests = append(rt.requests, req.Clone(req.Context()))
	path := req.URL.Path
	for _, rule := range rt.routes {
		if strings.Contains(path, rule.pathContains) {
			return cloneResponse(rule.response), nil
		}
	}
	// Default: 200 with an empty JSON object.
	return fakeResponse(http.StatusOK, `{}`, nil), nil
}

// lastSObjectRequest returns the most recent request whose path contains
// "/sobjects/" (i.e. the sObject REST request, not the /limits init call).
func (rt *routingTransport) lastSObjectRequest() *http.Request {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	for i := len(rt.requests) - 1; i >= 0; i-- {
		if strings.Contains(rt.requests[i].URL.Path, "/sobjects/") {
			return rt.requests[i]
		}
	}
	return nil
}

// cloneResponse creates a shallow copy of an http.Response with a fresh body
// reader so that the same response can be returned multiple times without
// exhausting the body reader.
func cloneResponse(resp *http.Response) *http.Response {
	if resp == nil {
		return fakeResponse(http.StatusOK, `{}`, nil)
	}
	// Read and re-buffer the body.
	var bodyStr string
	if resp.Body != nil {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck
		bodyStr = string(data)
		resp.Body = io.NopCloser(strings.NewReader(bodyStr))
	}
	clone := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       io.NopCloser(strings.NewReader(bodyStr)),
	}
	return clone
}

// fakeResponse builds a minimal *http.Response with the given status code,
// body, and optional headers.
func fakeResponse(statusCode int, body string, headers map[string]string) *http.Response {
	h := make(http.Header)
	for k, v := range headers {
		h.Set(k, v)
	}
	return &http.Response{
		StatusCode: statusCode,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// newRoutingTransport returns a routingTransport pre-configured with a 200
// handler for the /limits path used by sf.Init and a sobjects handler for the
// actual sObject REST call under test.
func newRoutingTransport(sobjectResponse *http.Response) *routingTransport {
	rt := &routingTransport{}
	rt.route("/limits", fakeResponse(http.StatusOK, `{}`, nil))
	rt.route("/sobjects/", sobjectResponse)
	return rt
}

// countingTransport is an http.RoundTripper used to test the 401 retry path.
// It returns firstResp on the first sObject request, retryResp on subsequent
// sObject requests, and limitsResp for /limits (sf.Init + refreshSession).
// callCount is incremented for each sObject request only.
type countingTransport struct {
	mu         sync.Mutex
	callCount  *int
	firstResp  *http.Response
	retryResp  *http.Response
	limitsResp *http.Response
}

func (ct *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if strings.Contains(req.URL.Path, "/limits") {
		return cloneResponse(ct.limitsResp), nil
	}
	// sObject request.
	*ct.callCount++
	if *ct.callCount == 1 {
		return cloneResponse(ct.firstResp), nil
	}
	return cloneResponse(ct.retryResp), nil
}

// ─── memCache ─────────────────────────────────────────────────────────────────

// memCache is a simple in-memory implementation of sObjectCacher for use in
// unit tests. It is safe for concurrent use.
type memCache struct {
	mu      sync.RWMutex
	entries map[string]*nats.SObjectCacheEntry
}

func newMemCache() *memCache {
	return &memCache{entries: make(map[string]*nats.SObjectCacheEntry)}
}

func (m *memCache) Get(_ context.Context, key string) (*nats.SObjectCacheEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.entries[key]
	if !ok {
		return nil, nil
	}
	return e, nil
}

func (m *memCache) Put(_ context.Context, key string, entry *nats.SObjectCacheEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = entry
	return nil
}

func (m *memCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}

// ─── TestFetchSObject ─────────────────────────────────────────────────────────

func TestFetchSObject(t *testing.T) {
	t.Parallel()

	const (
		sobjectType = "Account"
		sfid        = "001000000000001AAA"
		cacheKey    = "b2b_org.00000000-0000-0000-0000-000000000001"
		fields      = "Id,Name"
		sampleBody  = `{"Id":"001000000000001AAA","Name":"ACME Corp"}`
		sampleETag  = `"abc123"`
		sampleLM    = "Mon, 01 Jan 2024 00:00:00 GMT"
	)

	t.Run("cache miss: fetches from Salesforce and stores result", func(t *testing.T) {
		t.Parallel()

		transport := newRoutingTransport(fakeResponse(http.StatusOK, sampleBody, map[string]string{
			"ETag":          sampleETag,
			"Last-Modified": sampleLM,
		}))
		cache := newMemCache()
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: cache}

		result, err := client.FetchSObject(context.Background(), sobjectType, sfid, cacheKey, fields)

		require.NoError(t, err)
		assert.False(t, result.NotModified, "should be a fresh fetch")
		assert.Equal(t, sampleETag, result.ETag)
		assert.Equal(t, sampleLM, result.LastModified)
		assert.JSONEq(t, sampleBody, string(result.Body))

		// Cache must be populated after a 200.
		stored, err := cache.Get(context.Background(), cacheKey)
		require.NoError(t, err)
		require.NotNil(t, stored, "cache must contain the entry after a 200")
		assert.Equal(t, sampleETag, stored.ETag)
		assert.Equal(t, sampleLM, stored.LastModified)
		assert.JSONEq(t, sampleBody, string(stored.Body))

		// The request must not have included conditional headers.
		req := transport.lastSObjectRequest()
		require.NotNil(t, req, "must have made a sObject request")
		assert.Empty(t, req.Header.Get("If-None-Match"), "no ETag header on cache miss")
		assert.Empty(t, req.Header.Get("If-Modified-Since"), "no LM header on cache miss")
	})

	t.Run("304 Not Modified: returns cached body without re-writing cache", func(t *testing.T) {
		t.Parallel()

		// Pre-populate the cache so the client sends conditional headers.
		cache := newMemCache()
		cachedEntry := &nats.SObjectCacheEntry{
			ETag:         sampleETag,
			LastModified: sampleLM,
			Body:         json.RawMessage(sampleBody),
		}
		require.NoError(t, cache.Put(context.Background(), cacheKey, cachedEntry))

		// Salesforce returns 304 with an empty body. The go-salesforce library routes
		// non-2xx responses through processSalesforceError, which fails to unmarshal
		// the empty body and returns (resp, err). FetchSObject detects
		// resp.StatusCode == 304 and treats it as a cache hit.
		transport := newRoutingTransport(fakeResponse(http.StatusNotModified, "", nil))
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: cache}

		result, err := client.FetchSObject(context.Background(), sobjectType, sfid, cacheKey, fields)

		require.NoError(t, err)
		assert.True(t, result.NotModified, "304 should set NotModified flag")
		assert.Equal(t, sampleETag, result.ETag)
		assert.Equal(t, sampleLM, result.LastModified)
		assert.JSONEq(t, sampleBody, string(result.Body), "cached body must be returned on 304")

		// The request must have included conditional GET headers.
		req := transport.lastSObjectRequest()
		require.NotNil(t, req)
		assert.Equal(t, sampleETag, req.Header.Get("If-None-Match"), "ETag header must be forwarded")
		assert.Equal(t, sampleLM, req.Header.Get("If-Modified-Since"), "LM header must be forwarded")
	})

	t.Run("200 refresh with stale ETag: updates cache with new metadata", func(t *testing.T) {
		t.Parallel()

		const (
			newBody = `{"Id":"001000000000001AAA","Name":"ACME Corp Updated"}`
			newETag = `"def456"`
			newLM   = "Tue, 02 Jan 2024 00:00:00 GMT"
		)

		// Pre-populate with an old entry to simulate stale cache.
		cache := newMemCache()
		oldEntry := &nats.SObjectCacheEntry{
			ETag:         sampleETag,
			LastModified: sampleLM,
			Body:         json.RawMessage(sampleBody),
		}
		require.NoError(t, cache.Put(context.Background(), cacheKey, oldEntry))

		// Salesforce returns 200 with a different ETag (record changed).
		transport := newRoutingTransport(fakeResponse(http.StatusOK, newBody, map[string]string{
			"ETag":          newETag,
			"Last-Modified": newLM,
		}))
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: cache}

		result, err := client.FetchSObject(context.Background(), sobjectType, sfid, cacheKey, fields)

		require.NoError(t, err)
		assert.False(t, result.NotModified, "200 should not set NotModified flag")
		assert.Equal(t, newETag, result.ETag)
		assert.Equal(t, newLM, result.LastModified)
		assert.JSONEq(t, newBody, string(result.Body))

		// Cache must be refreshed with the new values.
		stored, err := cache.Get(context.Background(), cacheKey)
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, newETag, stored.ETag, "cache ETag must be updated")
		assert.Equal(t, newLM, stored.LastModified, "cache Last-Modified must be updated")
		assert.JSONEq(t, newBody, string(stored.Body), "cache body must be updated")

		// Conditional headers from the old entry must have been forwarded.
		req := transport.lastSObjectRequest()
		require.NotNil(t, req)
		assert.Equal(t, sampleETag, req.Header.Get("If-None-Match"))
		assert.Equal(t, sampleLM, req.Header.Get("If-Modified-Since"))
	})

	t.Run("validation: empty sobjectType returns error", func(t *testing.T) {
		t.Parallel()

		// Validation runs before any HTTP call; only the /limits route is needed.
		transport := &routingTransport{}
		transport.route("/limits", fakeResponse(http.StatusOK, `{}`, nil))
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}

		result, err := client.FetchSObject(context.Background(), "", sfid, cacheKey, fields)

		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("validation: empty sfid returns error", func(t *testing.T) {
		t.Parallel()

		transport := &routingTransport{}
		transport.route("/limits", fakeResponse(http.StatusOK, `{}`, nil))
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}

		result, err := client.FetchSObject(context.Background(), sobjectType, "", cacheKey, fields)

		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("validation: empty cacheKey returns error", func(t *testing.T) {
		t.Parallel()

		transport := &routingTransport{}
		transport.route("/limits", fakeResponse(http.StatusOK, `{}`, nil))
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}

		result, err := client.FetchSObject(context.Background(), sobjectType, sfid, "", fields)

		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("304 with no cached entry returns error", func(t *testing.T) {
		t.Parallel()

		// No conditional headers sent (empty cache), but Salesforce still returns
		// 304 — a protocol violation. FetchSObject must return an error.
		transport := newRoutingTransport(fakeResponse(http.StatusNotModified, "", nil))
		cache := newMemCache()
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: cache}

		result, err := client.FetchSObject(context.Background(), sobjectType, sfid, cacheKey, fields)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "304 Not Modified but no cached entry exists")
	})

	t.Run("401 triggers session refresh and retries once", func(t *testing.T) {
		t.Parallel()

		// The transport returns 401 on the first sObject request, then 200 on the
		// retry (simulating a successfully refreshed session). The /limits route
		// handles both sf.Init and the refreshSession no-op call.
		callCount := 0
		rt := &countingTransport{
			callCount: &callCount,
			firstResp: fakeResponse(http.StatusUnauthorized,
				`[{"errorCode":"INVALID_SESSION_ID","message":"Session expired"}]`, nil),
			retryResp: fakeResponse(http.StatusOK, sampleBody, map[string]string{
				"ETag":          sampleETag,
				"Last-Modified": sampleLM,
			}),
			limitsResp: fakeResponse(http.StatusOK, `{}`, nil),
		}
		cache := newMemCache()
		client := &SObjectClient{sf: fakeSalesforce(t, rt), cache: cache}

		result, err := client.FetchSObject(context.Background(), sobjectType, sfid, cacheKey, fields)

		require.NoError(t, err)
		assert.False(t, result.NotModified)
		assert.Equal(t, sampleETag, result.ETag)
		assert.JSONEq(t, sampleBody, string(result.Body))
		assert.Equal(t, 2, callCount, "exactly two sObject requests (initial + retry)")
	})
}

// ─── TestDoConditionalWrite ───────────────────────────────────────────────────

func TestDoConditionalWrite(t *testing.T) {
	t.Parallel()

	const (
		uri            = "/services/data/v63.0/sobjects/Account/001000000000001AAA"
		ifMatchValue   = `"abc123"`
		ifUnmodified   = "Mon, 01 Jan 2024 00:00:00 GMT"
		requestPayload = `{"Name":"ACME Corp Updated"}`
	)

	t.Run("forwards If-Match and If-Unmodified-Since headers", func(t *testing.T) {
		t.Parallel()

		transport := newRoutingTransport(fakeResponse(http.StatusOK, `{"id":"001000000000001AAA","success":true}`, nil))
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}

		resp, err := client.DoConditionalWrite(
			context.Background(),
			http.MethodPatch,
			uri,
			[]byte(requestPayload),
			ifMatchValue,
			ifUnmodified,
		)

		require.NoError(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close() //nolint:errcheck

		req := transport.lastSObjectRequest()
		require.NotNil(t, req)
		assert.Equal(t, ifMatchValue, req.Header.Get("If-Match"), "If-Match must be forwarded")
		assert.Equal(t, ifUnmodified, req.Header.Get("If-Unmodified-Since"), "If-Unmodified-Since must be forwarded")
	})

	t.Run("omits precondition headers when empty strings provided", func(t *testing.T) {
		t.Parallel()

		transport := newRoutingTransport(fakeResponse(http.StatusOK, `{"id":"001000000000001AAA","success":true}`, nil))
		client := &SObjectClient{sf: fakeSalesforce(t, transport), cache: newMemCache()}

		resp, err := client.DoConditionalWrite(
			context.Background(),
			http.MethodPatch,
			uri,
			[]byte(requestPayload),
			"", // no If-Match
			"", // no If-Unmodified-Since
		)

		require.NoError(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close() //nolint:errcheck

		req := transport.lastSObjectRequest()
		require.NotNil(t, req)
		assert.Empty(t, req.Header.Get("If-Match"), "If-Match must not be set when empty")
		assert.Empty(t, req.Header.Get("If-Unmodified-Since"), "If-Unmodified-Since must not be set when empty")
	})
}

// ─── TestInvalidateCache ──────────────────────────────────────────────────────

func TestInvalidateCache(t *testing.T) {
	t.Parallel()

	const cacheKey = "b2b_org.00000000-0000-0000-0000-000000000001"

	// limitsOnlyTransport returns a transport that handles only the /limits
	// initialisation call — InvalidateCache tests never make sObject requests.
	limitsOnlyTransport := func(t *testing.T) http.RoundTripper {
		t.Helper()
		rt := &routingTransport{}
		rt.route("/limits", fakeResponse(http.StatusOK, `{}`, nil))
		return rt
	}

	t.Run("removes existing entry", func(t *testing.T) {
		t.Parallel()

		cache := newMemCache()
		require.NoError(t, cache.Put(context.Background(), cacheKey, &nats.SObjectCacheEntry{
			ETag: `"abc"`,
			Body: json.RawMessage(`{}`),
		}))

		client := &SObjectClient{sf: fakeSalesforce(t, limitsOnlyTransport(t)), cache: cache}
		err := client.InvalidateCache(context.Background(), cacheKey)

		require.NoError(t, err)

		stored, err := cache.Get(context.Background(), cacheKey)
		require.NoError(t, err)
		assert.Nil(t, stored, "entry must be absent after invalidation")
	})

	t.Run("no error when key does not exist", func(t *testing.T) {
		t.Parallel()

		client := &SObjectClient{sf: fakeSalesforce(t, limitsOnlyTransport(t)), cache: newMemCache()}
		err := client.InvalidateCache(context.Background(), "nonexistent.key")
		require.NoError(t, err)
	})
}
