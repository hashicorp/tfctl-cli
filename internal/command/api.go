// Package command implements the tfcloud CLI command tree.
package command

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
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/tfcloud/internal/client"
	"github.com/hashicorp/tfcloud/internal/cmd"
	"github.com/hashicorp/tfcloud/internal/flagvalue"
	"github.com/hashicorp/tfcloud/internal/heredoc"
	"github.com/hashicorp/tfcloud/internal/iostreams"
	"github.com/hashicorp/tfcloud/internal/render"
)

type apiRequester interface {
	Base() *url.URL
	RawRequest(context.Context, *client.Request) (*client.Response, error)
}

type realAPIClient struct {
	client *client.Client
}

func (c *realAPIClient) Base() *url.URL {
	return c.client.BaseURL
}

func (c *realAPIClient) RawRequest(ctx context.Context, req *client.Request) (*client.Response, error) {
	return c.client.RawRequest(ctx, req)
}

// APIOpts stores the options parsed from flags for the API command.
type APIOpts struct {
	IO           iostreams.IOStreams
	Headers      []string
	Attributes   map[string]string
	Query        map[string]string
	PathTokens   map[string]string
	InputRequest string
	Method       string
	ResourceType string
	Paginate     bool
}

// NewCmdAPI creates the `tfcloud api` command.
func NewCmdAPI(ctx *cmd.Context) *cmd.Command {
	opts := &APIOpts{
		IO: ctx.IO,
	}

	cmd := &cmd.Command{
		Name:           "api",
		NoAuthRequired: true,
		ShortHelp:      "Perform any API request",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud api" }} command performs any API v2 request.
		`),
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "PATH",
					Documentation: "The API path to request, ex. /account/details. Unless -a or -i is used, the command will perform a GET request.",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:         "header",
					Shorthand:    "H",
					DisplayValue: "'name: value'",
					Description:  "Request header",
					Repeatable:   true,
					Value:        flagvalue.SimpleSlice(nil, &opts.Headers),
				},
				{
					Name:         "input",
					Shorthand:    "i",
					DisplayValue: "BODY",
					Description:  "Raw JSON request body (or - to read from stdin)",
					Value:        flagvalue.Simple("", &opts.InputRequest),
				},
				{
					Name:         "method",
					Shorthand:    "X",
					DisplayValue: "METHOD",
					Description:  "HTTP method to use (e.g. GET, POST, etc.)",
					Value:        flagvalue.Simple("", &opts.Method),
				},
				{
					Name:         "type",
					Shorthand:    "t",
					DisplayValue: "JSON:API TYPE",
					Description:  "Resource type for --attribute JSON:API request bodies. This value is inferred from the path whenever possible.",
					Value:        flagvalue.Simple("", &opts.ResourceType),
				},
				{
					Name:          "paginate",
					Description:   "Automatically paginate through results and stream them, one resource at a time. Only applies to successful responses with JSON:API document bodies.",
					Value:         flagvalue.Simple(false, &opts.Paginate),
					IsBooleanFlag: true,
				},
				{
					Name:         "attribute",
					Shorthand:    "a",
					DisplayValue: "ATTRIBUTE=VALUE",
					Description:  "Attribute for JSON:API request bodies. Implies POST method.",
					Repeatable:   true,
					Value:        flagvalue.SimpleMap(nil, &opts.Attributes),
				},
				{
					Name:         "field",
					Shorthand:    "f",
					DisplayValue: "KEY=VALUE",
					Description:  "Add a query parameter to the request URL",
					Repeatable:   true,
					Value:        flagvalue.SimpleMap(nil, &opts.Query),
				},
				{
					Name:         "pathtoken",
					Shorthand:    "p",
					DisplayValue: "TOKEN=NAME",
					Description:  "Resolve a path {token} with the given name. For example, --pathtoken 'workspace=foo' would replace {workspace} in the path with the ID of the foo workspace.",
					Repeatable:   true,
					Value:        flagvalue.SimpleMap(nil, &opts.PathTokens),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "List workspaces in the default organization",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud api /organizations/{organization}/workspaces`),
			},
			{
				Preamble: "Create a project using attributes",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud api /projects -a name=myproject`),
			},
			{
				Preamble: "Add remote state consumer",
				Command: heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud api /workspaces/{workspace}/remote-state-consumers -p 'workspace=my-workspace' -i '{ "data: [
	{
		"type":"remote-state-consumers",
		"id": "ws-glkT5DSQKuY8pAJ"
	}
]}'`),
			},
			{
				Preamble: "Create a workspace variable using a JSON:API request body",
				Command: heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Must(`$ tfcloud api /vars -i '{ "data": {
	"type":"vars",
	"attributes": {
		"key":"AWS_ACCESS_KEY_ID",
		"value":"FOOBARBAZQUX",
		"category":"env",
		"sensitive":true
	},
	"relationships": {
		"workspace": {
			"data": {
				"id":"ws-mjAtT5DSQKuY8pAJ",
				"type":"workspaces"
			}
		}
	}
}}'`),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			// TODO: replace `return err` statements with something that can be shown to the user.
			if len(args) < 1 {
				return cmd.ErrDisplayUsage
			}

			path := args[0]

			resolvedURL, err := client.ResolveURL(ctx.APIClient.BaseURL, path)
			if err != nil {
				return err
			}

			for _, item := range opts.Query {
				key, value, err := splitPair(item, '=')
				if err != nil {
					return err
				}
				query := resolvedURL.Query()
				query.Set(key, value)
				resolvedURL.RawQuery = query.Encode()
			}

			body, contentType, err := buildRequestBody(path, opts.InputRequest, opts.Attributes, opts.ResourceType, ctx.IO.In())
			if err != nil {
				return err
			}

			method := inferMethod(opts.Method, len(opts.Attributes) > 0, opts.InputRequest != "")
			requestHeaders, err := parseHeaders(opts.Headers)
			if err != nil {
				return err
			}
			if contentType != "" && requestHeaders.Get("Content-Type") == "" {
				requestHeaders.Set("Content-Type", contentType)
			}
			if requestHeaders.Get("Accept") == "" {
				requestHeaders.Set("Accept", "application/vnd.api+json")
			}

			response, err := ctx.APIClient.RawRequest(context.Background(), &client.Request{
				Method:  method,
				URL:     resolvedURL,
				Headers: requestHeaders,
				Body:    body,
			})
			if err != nil {
				return err
			}

			verbose := false
			if ctx.Profile.GetVerbosity() == "debug" || ctx.Profile.GetVerbosity() == "trace" {
				logRequestResponse(ctx.IO.Err(), method, resolvedURL, requestHeaders, response)
				verbose = true
			}

			if opts.Paginate && response.StatusCode >= 200 && response.StatusCode < 300 {
				response, err = paginateResponse(context.Background(), &realAPIClient{client: ctx.APIClient}, response, requestHeaders, verbose, ctx.IO.Err())
				if err != nil {
					return err
				}
			}

			if response.Headers.Get("Content-Type") != "" && strings.Contains(response.Headers.Get("Content-Type"), "text/html") {
				return errors.New("an HTML response was received, likely an error page")
			}

			if response.StatusCode < 200 || response.StatusCode >= 300 {
				message := summarizeAPIErrors(response.Body)
				if message == "" {
					message = string(bytes.TrimSpace(response.Body))
				}
				if message != "" {
					return fmt.Errorf("%s: %s", response.Status, message)
				}
				return errors.New(response.Status)
			}

			if ctx.Profile.IsQuiet() || len(bytes.TrimSpace(response.Body)) == 0 {
				return nil
			}

			// TODO: output should be determined by global flags and the ctx should
			// contain the displayer output device. This thing shoulld just write a data structure
			// to the displayer and let it handle formatting and output.
			table, ok, err := render.JSONAPITable(response.Body)
			if err != nil {
				return err
			}
			if ok {
				_, _ = ctx.IO.Out().Write([]byte(table))
				return nil
			}

			_, _ = ctx.IO.Out().Write(response.Body)
			return nil
		},
	}

	return cmd
}

func inferMethod(explicit string, hasAttributes, hasInput bool) string {
	if explicit != "" {
		return strings.ToUpper(explicit)
	}
	if hasAttributes || hasInput {
		return http.MethodPost
	}
	return http.MethodGet
}

func inferResourceType(path string) string {
	segments := strings.FieldsFunc(strings.Trim(path, "/"), func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return ""
	}
	last := segments[len(segments)-1]
	if len(segments) >= 2 {
		prev := segments[len(segments)-2]
		if !looksLikeCollection(last) && looksLikeCollection(prev) {
			return prev
		}
	}
	return last
}

func looksLikeCollection(segment string) bool {
	return strings.HasSuffix(segment, "s")
}

func parseTypedValue(raw string) any {
	if raw == "null" {
		return nil
	}
	if raw == "true" || raw == "false" {
		return raw == "true"
	}
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			return value
		}
	}
	return raw
}

func buildRequestBody(path, input string, attrs map[string]string, resourceType string, stdin io.Reader) ([]byte, string, error) {
	if input != "" {
		var data []byte
		var err error
		if input == "-" {
			data, err = io.ReadAll(stdin)
		} else if strings.HasPrefix(input, "{") || strings.HasPrefix(input, "[") {
			data = []byte(input)
		} else {
			data, err = os.ReadFile(input)
		}
		if err != nil {
			return nil, "", err
		}
		return data, "application/vnd.api+json", nil
	}

	if len(attrs) == 0 {
		return nil, "", nil
	}

	if resourceType == "" {
		resourceType = inferResourceType(path)
	}
	if resourceType == "" {
		return nil, "", errors.New("could not infer resource type from path; use --type")
	}

	attributes := make(map[string]any, len(attrs))
	for key, value := range attrs {
		attributes[key] = parseTypedValue(value)
	}

	body := map[string]any{
		"data": map[string]any{
			"type":       resourceType,
			"attributes": attributes,
		},
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}
	return encoded, "application/vnd.api+json", nil
}

func parseHeaders(values []string) (http.Header, error) {
	headers := make(http.Header)
	for _, item := range values {
		key, value, err := splitPair(item, ':')
		if err != nil {
			return nil, err
		}
		headers.Add(key, strings.TrimSpace(value))
	}
	return headers, nil
}

func splitPair(item string, sep rune) (string, string, error) {
	parts := strings.SplitN(item, string(sep), 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", fmt.Errorf("invalid pair %q", item)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func paginateResponse(ctx context.Context, apiClient apiRequester, initial *client.Response, headers http.Header, verbose bool, stderr io.Writer) (*client.Response, error) {
	combined, nextURL, err := parsePaginationPayload(initial.Body)
	if err != nil || nextURL == nil {
		return initial, err
	}

	for len(combined) < 1000 && nextURL != nil {
		resp, reqErr := apiClient.RawRequest(ctx, &client.Request{
			Method:  http.MethodGet,
			URL:     nextURL,
			Headers: headers,
		})
		if reqErr != nil {
			return nil, reqErr
		}
		if verbose {
			logRequestResponse(stderr, http.MethodGet, nextURL, headers, resp)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return resp, nil
		}

		pageData, pageNext, pageErr := parsePaginationPayload(resp.Body)
		if pageErr != nil {
			return nil, pageErr
		}
		combined = append(combined, pageData...)
		nextURL = pageNext
		if len(combined) > 1000 {
			combined = combined[:1000]
		}
		initial = resp
	}

	merged, err := mergePaginatedBody(initial.Body, combined)
	if err != nil {
		return nil, err
	}
	initial.Body = merged
	return initial, nil
}

func parsePaginationPayload(body []byte) ([]any, *url.URL, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}
	data, ok := payload["data"].([]any)
	if !ok {
		return nil, nil, nil
	}
	links, ok := payload["links"].(map[string]any)
	if !ok {
		return data, nil, nil
	}
	nextRaw, _ := links["next"].(string)
	if nextRaw == "" {
		return data, nil, nil
	}
	nextURL, err := url.Parse(nextRaw)
	if err != nil {
		return nil, nil, err
	}
	return data, nextURL, nil
}

func mergePaginatedBody(body []byte, combined []any) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["data"] = combined
	if meta, ok := payload["meta"].(map[string]any); ok {
		if pagination, ok := meta["pagination"].(map[string]any); ok {
			pagination["total-count"] = len(combined)
		}
	}
	if links, ok := payload["links"].(map[string]any); ok {
		links["next"] = nil
	}
	return json.Marshal(payload)
}

func summarizeAPIErrors(body []byte) string {
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

func logRequestResponse(w io.Writer, method string, u *url.URL, reqHeaders http.Header, response *client.Response) {
	fmt.Fprintf(w, "> %s %s\n", method, u.String())
	writeHeaders(w, reqHeaders)
	fmt.Fprintf(w, "< %s\n", response.Status)
	writeHeaders(w, response.Headers)
}

func writeHeaders(w io.Writer, headers http.Header) {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(w, "%s: %s\n", key, strings.Join(headers.Values(key), ", "))
	}
}
