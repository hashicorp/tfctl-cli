// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package api implements the tfctl CLI API command.
package api

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

	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/openapi"
)

const (
	// MaxPaginateRecords is the maximum number of records that will be returned
	// when using the --paginate argument, regardless of how many records are available.
	MaxPaginateRecords = 2000
)

// Opts stores the options parsed from flags for the API command.
type Opts struct {
	IO           iostreams.IOStreams
	Output       *format.Outputter
	ShutdownCtx  context.Context
	Client       *client.Client
	Quiet        bool
	Debug        bool
	DryRun       bool
	Headers      []string
	URL          *url.URL
	Attributes   map[string]string
	Query        map[string]string
	PathTokens   map[string]string
	InputRequest string
	Method       string
	ResourceType string
	All          bool
	PageSize     int
	PageNumber   int
}

// NewCmdAPI creates the `api` command.
func NewCmdAPI(ctx *cmd.Context) *cmd.Command {
	opts := &Opts{
		IO:          ctx.IO,
		ShutdownCtx: ctx.ShutdownCtx,
		Output:      ctx.Output,
	}

	oas, oaserr := openapi.SchemaFactory(nil)
	if oaserr != nil {
		fmt.Fprintf(ctx.IO.Err(), "%s failed to load embedded openAPI spec, this is always a %s bug", ctx.IO.ColorScheme().ErrorLabel(), config.Name)
		panic(oaserr)
	}

	cmd := &cmd.Command{
		Name:      "api",
		ShortHelp: "Perform any API request",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s api" }} command performs any API v2 request.
		`, config.Name),
		Args: cmd.PositionalArguments{
			// Predict paths from the OpenAPI spec for autocompletion.
			Autocomplete: complete.PredictSet(oas.Paths().Keys()...),
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
					Name:          "all",
					Description:   fmt.Sprintf("Fetch all records. May be slow for large amounts of data. Limited to %d records.", MaxPaginateRecords),
					Value:         flagvalue.Simple(false, &opts.All),
					IsBooleanFlag: true,
				},
				{
					Name:        "page-size",
					Description: "Limit the number of records to return. Default varies by resource. Ignored if --all is set.",
					Value:       flagvalue.Simple(0, &opts.PageSize), // page size is determined by the server, so we don't set it by default
				},
				{
					Name:        "page-number",
					Description: "Page number to return. Ignored if --all is set. Default is 1.",
					Value:       flagvalue.Simple(1, &opts.PageNumber),
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
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /organizations/{organization}/workspaces`, config.Name),
			},
			{
				Preamble: "Create a project using attributes",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /projects -a name=myproject`, config.Name),
			},
			{
				Preamble: "Add remote state consumer",
				Command: heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /workspaces/{workspace}/remote-state-consumers -p 'workspace=my-workspace' -i '{ "data": [
	{
		"type":"remote-state-consumers",
		"id": "ws-glkT5DSQKuY8pAJ"
	}
]}'`, config.Name),
			},
			{
				Preamble: "Create a workspace variable using a JSON:API request body",
				Command: heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /vars -i '{ "data": {
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
}}'`, config.Name),
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			if len(args) < 1 {
				return cmd.ErrDisplayUsage
			}

			path := args[0]

			apiClient, err := ctx.NewAPIClient()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			resolvedURL, err := client.ResolveURL(*apiClient.BaseURL, path)
			if err != nil {
				return fmt.Errorf("invalid input path/URL %q", path)
			}

			opts.URL = resolvedURL
			opts.Client = apiClient

			opts.Debug = ctx.Profile.GetVerbosity() == "debug" || ctx.Profile.GetVerbosity() == "trace"
			opts.Quiet = ctx.Profile.IsQuiet()
			opts.DryRun = ctx.IsDryRun()

			return runAPI(opts)
		},
	}

	cmd.AddChild(NewCmdAPISchema(ctx))

	return cmd
}

func runAPI(opts *Opts) error {
	// Handle -f query fields
	query := opts.URL.Query()
	for key, value := range opts.Query {
		query.Set(key, value)
	}

	// Handle pagination parameters unless --all is set.
	if opts.PageNumber > 1 {
		if opts.All {
			fmt.Fprintf(opts.IO.Err(), "%s ignoring --page-number because --all is set\n", opts.IO.ColorScheme().WarningLabel())
		} else {
			query.Set("page[number]", fmt.Sprintf("%d", opts.PageNumber))
		}
	}

	if opts.PageSize > 0 {
		if opts.All {
			fmt.Fprintf(opts.IO.Err(), "%s ignoring --page-size because --all is set\n", opts.IO.ColorScheme().WarningLabel())
		} else {
			query.Set("page[size]", fmt.Sprintf("%d", opts.PageSize))
		}
	}

	opts.URL.RawQuery = query.Encode()

	// Construct a request
	body, contentType, err := buildRequestBody(opts.URL.Path, opts.InputRequest, opts.Attributes, opts.ResourceType, opts.IO.In())
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

	// Interactive prompt required for DELETE requests to prevent accidental data loss
	if method == http.MethodDelete {
		if opts.Quiet {
			return errors.New("can't perform DELETE request confirmation with quiet mode enabled")
		}
		if !opts.IO.CanPrompt() {
			return errors.New("can't perform DELETE request without confirmation in non-interactive mode")
		}

		dryRunWarning := ""
		if opts.DryRun {
			dryRunWarning = " (no actual request will be sent in dry-run mode)"
		}

		confirmation, err := opts.IO.PromptConfirm(fmt.Sprintf("The request must be confirmed because it is a DELETE action%s.\n\nDo you want to continue", dryRunWarning))
		if err != nil {
			return fmt.Errorf("failed to confirm DELETE request: %w", err)
		}
		if !confirmation {
			return errors.New("DELETE request canceled")
		}
	}

	// In dry-run mode, skip mutating requests and report what would have happened.
	if opts.DryRun && isMutationMethod(method) {
		fmt.Fprintf(opts.IO.Err(), "%s would send %s request\n", opts.IO.ColorScheme().DryRunLabel(), method)
		writeDryRunRequest(opts.IO.Err(), method, opts.URL, requestHeaders, body)
		return nil
	}

	// Make the request
	response, err := opts.Client.RawRequest(opts.ShutdownCtx, &client.Request{
		Method:  method,
		URL:     opts.URL,
		Headers: requestHeaders,
		Body:    body,
	})
	if err != nil {
		return err
	}

	verbose := false
	if opts.Debug {
		logRequestResponse(opts.IO.Err(), method, opts.URL, requestHeaders, response)
		verbose = true
	}

	if opts.All && response.StatusCode >= 200 && response.StatusCode < 300 {
		response, err = paginateResponse(opts.ShutdownCtx, opts.Client, response, requestHeaders, verbose, opts.IO.Err())
		if err != nil {
			return err
		}
	}

	if response.Headers.Get("Content-Type") != "" && strings.Contains(response.Headers.Get("Content-Type"), "text/html") {
		return errors.New("an HTML response was received, likely an error page")
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := client.SummarizeAPIErrors(response.Body)
		if message == "" {
			message = string(bytes.TrimSpace(response.Body))
		}
		if message != "" {
			return fmt.Errorf("%s: %s", response.Status, message)
		}
		return errors.New(response.Status)
	}

	if opts.Quiet || len(bytes.TrimSpace(response.Body)) == 0 {
		return nil
	}

	// Render the result
	disp, err := format.NewJSONAPIDisplayer(response.Body)
	if err != nil {
		return err
	}

	return opts.Output.Display(disp)
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

func paginateResponse(ctx context.Context, apiClient *client.Client, initial *client.Response, headers http.Header, verbose bool, stderr io.Writer) (*client.Response, error) {
	combined, nextURL, err := parsePaginationPayload(initial.Body)
	if err != nil || nextURL == nil {
		return initial, err
	}

	for len(combined) < MaxPaginateRecords && nextURL != nil {
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
		if len(combined) > MaxPaginateRecords {
			combined = combined[:MaxPaginateRecords]
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
			pagination["page-size"] = len(combined)
			pagination["current-page"] = 1
			pagination["total-pages"] = 1
			pagination["prev-page"] = nil
		}
	}
	if links, ok := payload["links"].(map[string]any); ok {
		links["next"] = nil
	}
	return json.Marshal(payload)
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

func isMutationMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func writeDryRunRequest(w io.Writer, method string, u *url.URL, headers http.Header, body []byte) {
	fmt.Fprintf(w, "> %s %s\n", method, u.String())
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(w, "> %s: %s\n", key, strings.Join(headers.Values(key), ", "))
	}
	if len(body) == 0 {
		return
	}
	fmt.Fprintln(w)
	_, _ = w.Write(formatDryRunBody(body))
	fmt.Fprintln(w)
}

func formatDryRunBody(body []byte) []byte {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, body, "", "  "); err == nil {
		return formatted.Bytes()
	}
	return body
}
