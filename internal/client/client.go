// Package client provides configured HCP Terraform API clients and raw request helpers.
package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/brandonc/tfcloud/internal/config"
	tfe "github.com/hashicorp/go-tfe"
	abs "github.com/microsoft/kiota-abstractions-go"
)

// Client wraps the configured HCP Terraform API clients and request helpers.
type Client struct {
	// TFE is the underlying go-tfe client.
	TFE *tfe.Client
	// HTTP is the shared HTTP client used for raw requests.
	HTTP *http.Client
	// Adapter is the Kiota request adapter from the go-tfe client.
	Adapter abs.RequestAdapter
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

// New constructs a configured API client from CLI configuration.
func New(cfg *config.Config) (*Client, error) {
	tfeClient, err := tfe.NewClient(&tfe.Config{
		Address: fmt.Sprintf("https://%s", cfg.Hostname),
		Token:   cfg.Token,
		Headers: cfg.DefaultHeaders,
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
		HTTP:           native.Client,
		Adapter:        adapter,
		BaseURL:        &baseURL,
		DefaultHeaders: cfg.DefaultHeaders,
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

	httpResp, err := c.HTTP.Do(httpReq)
	if err != nil {
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
func ResolveURL(base *url.URL, path string) (*url.URL, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return url.Parse(path)
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	resolved := *base
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
