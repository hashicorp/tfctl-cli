// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package client provides configured HCP Terraform API clients and raw request helpers.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	tfe "github.com/hashicorp/go-tfe"
	abs "github.com/microsoft/kiota-abstractions-go"
)

// Client wraps the configured HCP Terraform API clients and request helpers.
type Client struct {
	// TFE is the underlying go-tfe client.
	TFE *tfe.Client
	// HTTP is the shared HTTP client used for raw requests.
	Adapter *tfe.TFERequestAdapter
	// BaseURL is the resolved API base URL.
	BaseURL *url.URL
	// DefaultHeaders are applied to every request.
	DefaultHeaders http.Header
}

// Request describes a raw HTTP request to send to the API.
type Request struct {
	// Method is the HTTP method to use.
	Method string
	// URL is the fully resolved request URL.
	URL *url.URL
	// Headers are additional HTTP headers for the request.
	Headers http.Header
	// Body is the raw request payload.
	Body []byte
}

// New constructs a configured API client from an API address and token.
// If logger is non-nil, retry attempts are logged at debug level.
func New(address, token string, defaultHeaders http.Header, logger hclog.Logger) (*Client, error) {
	cfg := &tfe.Config{
		Address:           address,
		Token:             token,
		Headers:           defaultHeaders,
		RetryServerErrors: true,
		RetryRateLimited:  true,
		RetryMaxRetries:   5,
	}
	cfg.RetryHook = func(attemptNum int, resp *http.Response) {
		status := 0
		url := ""
		method := ""
		if resp != nil {
			status = resp.StatusCode
			if resp.Request != nil {
				url = resp.Request.URL.String()
				method = resp.Request.Method
			}
		}
		logger.Debug("Retrying API request", "attempt", attemptNum, "status", status, "method", method, "url", url)
	}
	tfeClient, err := tfe.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	adapter := tfeClient.API.RequestAdapter
	native, ok := adapter.(*tfe.TFERequestAdapter)
	if !ok {
		return nil, fmt.Errorf("unsupported request adapter type %T", adapter)
	}

	baseURL := tfeClient.BaseURL()
	return &Client{
		TFE:            tfeClient,
		Adapter:        native,
		BaseURL:        &baseURL,
		DefaultHeaders: defaultHeaders,
	}, nil
}

// Do sends a low-level request and returns the response. It is the callers responsibility to close
// the response body.
func (c *Client) Do(ctx context.Context, req *Request) (*http.Response, error) {
	requestInfo := abs.NewRequestInformation()
	requestInfo.Method = httpMethod(strings.ToUpper(req.Method))
	requestInfo.SetUri(*req.URL)

	for key, values := range c.DefaultHeaders {
		for _, value := range values {
			requestInfo.Headers.Add(key, value)
		}
	}
	for key, values := range req.Headers {
		for _, value := range values {
			requestInfo.Headers.Add(key, value)
		}
	}
	if len(req.Body) > 0 {
		requestInfo.Content = req.Body
	}

	nativeRequest, err := c.Adapter.ConvertToNativeRequest(ctx, requestInfo)
	if err != nil {
		return nil, err
	}

	httpReq, ok := nativeRequest.(*http.Request)
	if !ok {
		return nil, fmt.Errorf("unexpected native request type %T", nativeRequest)
	}

	httpResp, err := c.Adapter.Client.Do(httpReq)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, urlErr.Err
		}
		return nil, err
	}

	if httpResp.StatusCode >= 400 {
		if apiErr := tfe.APIErrorFactory(httpResp, nil); apiErr != nil {
			return nil, apiErr
		}
	}

	return httpResp, nil
}

// ResolveURL resolves an absolute or base-relative API path against base.
// Query parameters and fragments in path are preserved.
func ResolveURL(base url.URL, path string) (*url.URL, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return url.Parse(path)
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	parsed, err := url.Parse(path)
	if err != nil {
		return nil, err
	}

	resolved := base
	basePath := strings.TrimRight(base.Path, "/")
	resolved.Path = basePath + parsed.Path
	resolved.RawPath = "" // clear any base RawPath; re-set below if needed
	resolved.RawQuery = parsed.RawQuery
	resolved.Fragment = parsed.Fragment

	// Preserve RawPath so percent-encoded separators (e.g. %2F) are not
	// decoded into literal slashes, which would alter the path hierarchy.
	if parsed.RawPath != "" {
		resolved.RawPath = basePath + parsed.RawPath
	}

	return &resolved, nil
}

func httpMethod(method string) abs.HttpMethod {
	switch method {
	case http.MethodDelete:
		return abs.DELETE
	case http.MethodPatch:
		return abs.PATCH
	case http.MethodPost:
		return abs.POST
	case http.MethodPut:
		return abs.PUT
	default:
		return abs.GET
	}
}

// SummarizeAPIErrors attempts to extract meaningful error messages from typical API error responses.
func SummarizeAPIErrors(body []byte) string {
	var payload struct {
		Errors []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if len(payload.Errors) > 0 {
		parts := make([]string, 0, len(payload.Errors))
		for _, item := range payload.Errors {
			if item.Detail != "" {
				parts = append(parts, strings.TrimSpace(item.Title+": "+item.Detail))
				continue
			}
			if item.Title != "" {
				parts = append(parts, item.Title)
			}
		}
		return strings.Join(parts, ", ")
	}
	if payload.Message != "" {
		return payload.Message
	}
	return payload.Error
}

// SetLogger wraps the HTTP transport to log all requests and responses
// at the debug level.
func (c *Client) SetLogger(logger hclog.Logger) {
	if logger == nil {
		return
	}

	c.Adapter.Client.Transport = &loggingTransport{
		inner:  c.Adapter.Client.Transport,
		logger: logger,
	}
}

// loggingTransport wraps an http.RoundTripper to log every request and response.
type loggingTransport struct {
	inner  http.RoundTripper
	logger hclog.Logger
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	startTime := time.Now()
	t.logger.Debug("HTTP request", "method", req.Method, "url", req.URL.String())
	resp, err := t.inner.RoundTrip(req)
	duration := time.Since(startTime)

	if resp != nil {
		t.logger.Debug("HTTP response", "status", resp.Status, "content-type", resp.Header.Get("content-type"), "content-length", resp.ContentLength, "duration_ms", duration.Milliseconds())
	} else if err != nil {
		t.logger.Debug("HTTP request error", "error", err)
	}
	return resp, err
}
