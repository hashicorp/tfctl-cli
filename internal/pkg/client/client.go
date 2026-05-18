// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package client provides configured HCP Terraform API clients and raw request helpers.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Response contains the result of a raw HTTP request.
type Response struct {
	// StatusCode is the numeric HTTP status code.
	StatusCode int
	// Status is the full HTTP status line.
	Status string
	// Headers are the response headers.
	Headers http.Header
	// Body is the raw response body.
	Body []byte
}

// New constructs a configured API client from an API address and token.
func New(address, token string, defaultHeaders http.Header) (*Client, error) {
	tfeClient, err := tfe.NewClient(&tfe.Config{
		Address: address,
		Token:   token,
		Headers: defaultHeaders,
	})
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

// RawRequest sends a low-level request and returns the raw response.
func (c *Client) RawRequest(ctx context.Context, req *Request) (*Response, error) {
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
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: httpResp.StatusCode,
		Status:     httpResp.Status,
		Headers:    httpResp.Header.Clone(),
		Body:       body,
	}, nil
}

// ResolveURL resolves an absolute or base-relative API path against base.
func ResolveURL(base url.URL, path string) (*url.URL, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return url.Parse(path)
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	resolved := base
	resolved.Path = strings.TrimRight(base.Path, "/") + path
	resolved.RawQuery = ""
	resolved.Fragment = ""

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
		t.logger.Debug("HTTP response", "status", resp.Status, "content-type", resp.Header.Get("content-type"), "duration_ms", duration.Milliseconds())
	} else if err != nil {
		t.logger.Debug("HTTP request error", "error", err)
	}
	return resp, err
}
