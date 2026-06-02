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

	"github.com/hashicorp/go-hclog"
	"github.com/posener/complete"

	tfe "github.com/hashicorp/go-tfe"

	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/openapi"
	terraformcfg "github.com/hashicorp/tfctl-cli/internal/pkg/terraform"
	"github.com/hashicorp/tfctl-cli/version"
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
	Logger       hclog.Logger
	ShutdownCtx  context.Context
	Client       *client.Client
	Quiet        bool
	DryRun       bool
	Headers      []string
	URL          *url.URL
	Attributes   map[string]string
	Query        map[string]string
	PathParams   map[string]string
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

	// The embedded schema is always used for autocomplete
	oas := openapi.LoadEmbeddedSchema()

	cmd := &cmd.Command{
		Name:      "api",
		ShortHelp: "Perform any API request",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s api" }} command performs any HCP Terraform API v2 request.

		Use {name} placeholders for path parameters and -p to set their values:

		  tfcloud api /workspaces/{workspace}/runs -p workspace=my-workspace

		Organization resolves automatically from the active profile or local Terraform cloud config.
		Workspaces, teams, projects, and varsets resolve from name to ID. Values that
		already look like IDs (ws-, team-, prj-, varset- prefixes) are used directly.
		`, version.Name),
		Args: cmd.PositionalArguments{
			// Predict paths from the OpenAPI spec for autocompletion.
			Autocomplete: complete.PredictSet(oas.Paths().Keys()...),
			Args: []cmd.PositionalArgument{
				{
					Name:          "PATH",
					Documentation: "API path or URL. Supports {name} path parameter name resolution",
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
					Name:         "pathparam",
					Shorthand:    "p",
					DisplayValue: "KEY=VALUE",
					Description:  "Provide a hint for path parameter resolution. The TFE API typically requires a resource ID for resource-specific requests. Use of the --pathparam flag allows automatic resolution to resource ID from name (workspaces, teams, projects, varsets). This flag can be used to specify an organization and workspace, but these resource IDs will also be automatically resolved from the active profile or local Terraform config if either is present.",
					Repeatable:   true,
					Value:        flagvalue.SimpleMap(nil, &opts.PathParams),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "List workspaces in the active profile's organization",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /organizations/{organization}/workspaces`, version.Name),
			},
			{
				Preamble: "List runs for a workspace by name (resolved to ID)",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /workspaces/{workspace}/runs -p workspace=my-workspace`, version.Name),
			},
			{
				Preamble: "Use a known ID directly (no resolution)",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /workspaces/{workspace}/runs -p workspace=ws-abc123`, version.Name),
			},
			{
				Preamble: "Create a project using attributes",
				Command:  heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /projects -a name=myproject`, version.Name),
			},
			{
				Preamble: "Add remote state consumer",
				Command: heredoc.New(ctx.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s api /workspaces/{workspace}/remote-state-consumers -p 'workspace=my-workspace' -i '{ "data": [
	{
		"type":"remote-state-consumers",
		"id": "ws-glkT5DSQKuY8pAJ"
	}
]}'`, version.Name),
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
}}'`, version.Name),
			},
		},
		RunF: func(c *cmd.Command, args []string) error {
			if len(args) < 1 {
				return cmd.ErrDisplayUsage
			}

			path := args[0]

			logger := c.Logger(ctx)
			apiClient, err := ctx.NewAPIClient(logger)
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}

			// Resolve path params ({workspace}, {organization}, etc.) before URL resolution.
			if strings.Contains(path, "{") {
				resolvedPath, resolveErr := resolvePathParamsFromContext(ctx, apiClient, path, opts.PathParams)
				if resolveErr != nil {
					return resolveErr
				}
				path = resolvedPath
			}

			resolvedURL, err := client.ResolveURL(*apiClient.BaseURL, path)
			if err != nil {
				return fmt.Errorf("invalid input path/URL %q", path)
			}

			opts.URL = resolvedURL
			opts.Client = apiClient
			opts.Logger = logger
			opts.Quiet = ctx.Profile.IsQuiet()
			opts.DryRun = ctx.IsDryRun()

			return runAPI(opts)
		},
	}

	cmd.AddChild(NewCmdAPISchema(ctx))

	return cmd
}

// resolvePathParamsFromContext resolves {param} placeholders using the command context.
// Params preceded by a known resource segment (workspaces, teams, projects, varsets)
// are resolved from name to ID via the API.
func resolvePathParamsFromContext(ctx *cmd.Context, apiClient *client.Client, path string, pathParams map[string]string) (string, error) {
	if pathParams == nil {
		pathParams = make(map[string]string)
	}
	paramSegments := client.ParsePathParams(path)

	// Load terraform cloud config once for org/workspace auto-fill.
	cloudCfg, _ := terraformcfg.FindCloudConfig(".")

	// Auto-fill organization from profile or terraform config if not explicit.
	org := ""
	for param, segment := range paramSegments {
		if segment == "organizations" {
			if _, ok := pathParams[param]; !ok {
				if org == "" {
					org = resolveOrg(ctx, cloudCfg)
				}
				if org != "" {
					pathParams[param] = org
				}
			} else {
				org = pathParams[param]
			}
		}
	}
	if org == "" {
		org = resolveOrg(ctx, cloudCfg)
	}

	// Auto-fill workspace from terraform config if not explicit.
	for param, segment := range paramSegments {
		if segment == "workspaces" {
			if _, ok := pathParams[param]; !ok {
				if cloudCfg != nil && cloudCfg.Workspace != "" {
					pathParams[param] = cloudCfg.Workspace
				}
			}
		}
	}

	// Resolve names → IDs for params preceded by known resource segments.
	resolver := client.NewResolver(apiClient, false, false)
	for param, segment := range paramSegments {
		value, ok := pathParams[param]
		if !ok {
			continue
		}
		if !client.IsResolvableSegment(segment) {
			continue
		}
		if client.LooksLikeID(segment, value) {
			continue
		}
		if org == "" {
			return "", fmt.Errorf("organization required to resolve %s name %q; configure a profile or use -p with an organization param", segment, value)
		}
		id, err := lookupResource(ctx.ShutdownCtx, resolver, segment, org, value)
		if err != nil {
			return "", err
		}
		pathParams[param] = id
	}

	return client.ResolvePathParams(path, pathParams)
}

// resolveOrg returns the organization from profile or terraform cloud config.
func resolveOrg(ctx *cmd.Context, cloudCfg *terraformcfg.CloudConfig) string {
	if ctx.Profile.Organization != "" {
		return ctx.Profile.Organization
	}
	if cloudCfg != nil && cloudCfg.Organization != "" {
		return cloudCfg.Organization
	}
	return ""
}

// lookupResource resolves a resource name to its ID via the API.
func lookupResource(goCtx context.Context, resolver *client.Resolver, segment, org, name string) (string, error) {
	id, err := resolver.ResolveFromName(goCtx, segment, org, name)
	if err != nil {
		if errors.Is(err, tfe.ErrNotFound) || isNotFound(err) {
			return "", fmt.Errorf("%s named %q not found in organization %q", segment, name, org)
		}

		return "", err
	}
	if id == nil {
		return "", fmt.Errorf("%s %q resolved to nil ID", segment, name)
	}
	return *id, nil
}

// isNotFound checks whether an error from the Kiota SDK indicates a 404 response.
func isNotFound(err error) bool {
	var apiErr interface{ GetStatusCode() int }
	if errors.As(err, &apiErr) {
		return apiErr.GetStatusCode() == http.StatusNotFound
	}
	return false
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
	response, err := opts.Client.Do(opts.ShutdownCtx, &client.Request{
		Method:  method,
		URL:     opts.URL,
		Headers: requestHeaders,
		Body:    body,
	})

	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}

	if err != nil {
		// Non-success response codes are already decoded by the client
		return err
	}

	if opts.All && response.StatusCode >= 200 && response.StatusCode < 300 {
		response, err = paginateResponse(opts.ShutdownCtx, opts.Client, response, requestHeaders)
		if err != nil {
			return err
		}
		defer response.Body.Close()
	}

	if opts.Quiet {
		opts.Logger.Debug("Quiet mode enabled or no content to display, rendering skipped")
		return nil
	}

	if response.StatusCode == http.StatusNoContent {
		opts.Logger.Debug("No Content response, nothing to display")
		return nil
	}

	if response.ContentLength == 0 {
		opts.Logger.Debug("Empty response body, nothing to display")
		return nil
	}

	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/vnd.api+json") {
		opts.Logger.Debug("Response body was not application/vnd.api+json, rendering raw body")
		_, _ = io.Copy(opts.IO.Out(), response.Body)
		return nil
	}

	body, err = io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Render the result
	disp, err := format.NewJSONAPIDisplayer(body, opts.Logger)
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

func paginateResponse(ctx context.Context, apiClient *client.Client, initial *http.Response, headers http.Header) (*http.Response, error) {
	initialBody, err := io.ReadAll(initial.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	combined, nextURL, err := parsePaginationPayload(initialBody)
	if err != nil || nextURL == nil {

		return initial, err
	}

	for len(combined) < MaxPaginateRecords && nextURL != nil {
		resp, reqErr := apiClient.Do(ctx, &client.Request{
			Method:  http.MethodGet,
			URL:     nextURL,
			Headers: headers,
		})
		if reqErr != nil {
			return nil, reqErr
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() // Close intermediate response bodies immediately
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		pageData, pageNext, pageErr := parsePaginationPayload(body)
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

	merged, err := mergePaginatedBody(initialBody, combined)
	if err != nil {
		return nil, err
	}
	initial.Body = io.NopCloser(bytes.NewReader(merged))
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
