// Package command implements the tfcloud CLI command tree.
package command

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
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hashicorp/tfcloud/internal/client"
	"github.com/hashicorp/tfcloud/internal/config"
	"github.com/hashicorp/tfcloud/internal/render"
)

// APICommand performs arbitrary HCP Terraform API requests.
type APICommand struct {
	// Meta provides UI and stream access for command execution.
	Meta       *Meta
	loadConfig func() (*config.Config, error)
	newClient  func(*config.Config) (apiRequester, error)
}

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

type apiOutputMode int

const (
	apiOutputHuman apiOutputMode = iota
	apiOutputMachine
)

type apiStyles struct {
	header lipgloss.Style
	label  lipgloss.Style
	error  lipgloss.Style
	json   lipgloss.Style
}

const ansiHelpBoldWhite = "\x1b[1;97m"

// Synopsis returns a short summary of the command.
func (c *APICommand) Synopsis() string { return "Make arbitrary API requests" }

// Help returns the command help text.
func (c *APICommand) Help() string {
	help := strings.TrimSpace(`Usage: tfcloud api <path> [flags]

Perform an HCP Terraform API v2 request. Table output by default; use -json,
-agent or pipe output for JSON.

Options:

  -H, -header "key: value"   Add request header
  -i, -input file            Read raw JSON request body from file or - for stdin
  -X, -method method         HTTP method
  -t, -type type             Resource type for -attribute JSON:API bodies
  -paginate                  Follow links.next and combine up to 1000 resources
  -a, -attribute key=value   Add typed JSON:API request attribute
  -f, -field key=value       Add query parameter
  -silent                    Suppress response body output
  -agent, -json              Print JSON output
  -v, -verbose               Log request and response metadata to stderr
  -(path token) name         E.g. -organization myorg to replace {organization}

Path templates:

  Use tokens like {workspace} or {organization} in paths to have that token
  automatically replaced with the corresponding name given by flag or
  configuration

Examples:

  # List workspaces in the default organization
  $ tfcloud api /organizations/{organization}/workspaces

  # Create a project using attributes
  $ tfcloud api /projects -a name=myproject

  # Create a workspace variable using a JSON:API request body
  $ tfcloud api /vars -i '{ "data": {
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
    }}'
`)

	for _, heading := range []string{"Options:", "Path templates:", "Examples:"} {
		help = strings.Replace(help, heading, ansiHelpBoldWhite+heading+"\x1b[0m", 1)
	}
	return help
}

// Run executes the API command.
func (c *APICommand) Run(args []string) int {
	var headers multiFlag
	var attrs multiFlag
	var filters multiFlag
	var input string
	var method string
	var resourceType string
	var paginate bool
	var silent bool
	var rawJSON bool
	var verbose bool

	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(&headers, "H", "header")
	fs.Var(&headers, "header", "header")
	fs.StringVar(&input, "input", "", "input")
	fs.StringVar(&input, "i", "", "input")
	fs.StringVar(&method, "X", "", "method")
	fs.StringVar(&method, "method", "", "method")
	fs.StringVar(&resourceType, "t", "", "type")
	fs.StringVar(&resourceType, "type", "", "type")
	fs.BoolVar(&paginate, "paginate", false, "paginate")
	fs.Var(&attrs, "a", "attribute")
	fs.Var(&attrs, "attribute", "attribute")
	fs.Var(&filters, "f", "field")
	fs.Var(&filters, "field", "field")
	fs.BoolVar(&silent, "silent", false, "silent")
	fs.BoolVar(&rawJSON, "agent", false, "agent")
	fs.BoolVar(&rawJSON, "json", false, "json")
	fs.BoolVar(&verbose, "v", false, "verbose")
	fs.BoolVar(&verbose, "verbose", false, "verbose")

	path, err := parseSingleArg(args)
	if err != nil {
		c.Meta.UI.Error(err.Error())
		return 1
	}

	if err := fs.Parse(args[1:]); err != nil {
		c.Meta.UI.Error(err.Error())
		return 1
	}

	if len(attrs) > 0 && input != "" {
		c.Meta.UI.Error("-attribute and -input are mutually exclusive")
		return 1
	}

	loadConfig := c.loadConfig
	if loadConfig == nil {
		loadConfig = config.Load
	}

	cfg, err := loadConfig()
	if err != nil {
		c.emitError(err.Error())
		return 1
	}

	newClient := c.newClient
	if newClient == nil {
		newClient = func(cfg *config.Config) (apiRequester, error) {
			apiClient, err := client.New(cfg)
			if err != nil {
				return nil, err
			}
			return &realAPIClient{client: apiClient}, nil
		}
	}

	apiClient, err := newClient(cfg)
	if err != nil {
		c.emitError(err.Error())
		return 1
	}

	resolvedURL, err := client.ResolveURL(apiClient.Base(), path)
	if err != nil {
		c.emitError(err.Error())
		return 1
	}

	for _, item := range filters {
		key, value, err := splitPair(item, '=')
		if err != nil {
			c.emitError(err.Error())
			return 1
		}
		query := resolvedURL.Query()
		query.Set(key, value)
		resolvedURL.RawQuery = query.Encode()
	}

	body, contentType, err := buildRequestBody(path, input, attrs, resourceType, c.Meta.Stdin)
	if err != nil {
		c.emitError(err.Error())
		return 1
	}

	method = inferMethod(method, len(attrs) > 0, input != "")
	requestHeaders, err := parseHeaders(headers)
	if err != nil {
		c.emitError(err.Error())
		return 1
	}
	if contentType != "" && requestHeaders.Get("Content-Type") == "" {
		requestHeaders.Set("Content-Type", contentType)
	}
	if requestHeaders.Get("Accept") == "" {
		requestHeaders.Set("Accept", "application/vnd.api+json")
	}

	response, err := apiClient.RawRequest(context.Background(), &client.Request{
		Method:  method,
		URL:     resolvedURL,
		Headers: requestHeaders,
		Body:    body,
	})
	if err != nil {
		c.emitError(err.Error())
		return 1
	}

	if verbose {
		logRequestResponse(c.Meta.Stderr, method, resolvedURL, requestHeaders, response)
	}

	if paginate && response.StatusCode >= 200 && response.StatusCode < 300 {
		response, err = paginateResponse(context.Background(), apiClient, response, requestHeaders, verbose, c.Meta.Stderr)
		if err != nil {
			c.emitError(err.Error())
			return 1
		}
	}

	if response.Headers.Get("Content-Type") != "" && strings.Contains(response.Headers.Get("Content-Type"), "text/html") {
		c.emitError("HTML response received, likely an error page. Check the URL and try again.")
		return 1
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := summarizeAPIErrors(response.Body)
		if message == "" {
			message = string(bytes.TrimSpace(response.Body))
		}
		if message != "" {
			c.emitError(fmt.Sprintf("%s: %s", response.Status, message))
		} else {
			c.emitError(response.Status)
		}
		return 1
	}

	if silent || len(bytes.TrimSpace(response.Body)) == 0 {
		return 0
	}

	mode := c.outputMode(rawJSON)
	if rawJSON || mode == apiOutputMachine {
		c.emitOutput(c.renderJSON(response.Body, mode))
		return 0
	}

	table, ok, err := render.JSONAPITable(response.Body)
	if err != nil {
		c.emitError(err.Error())
		return 1
	}
	if ok {
		c.emitOutput(table)
		return 0
	}

	c.emitOutput(c.renderJSON(response.Body, mode))
	return 0
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
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

func buildRequestBody(path, input string, attrs multiFlag, resourceType string, stdin io.Reader) ([]byte, string, error) {
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
		return nil, "", errors.New("could not infer resource type from path; use -type")
	}

	attributes := make(map[string]any, len(attrs))
	for _, item := range attrs {
		key, value, err := splitPair(item, '=')
		if err != nil {
			return nil, "", err
		}
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

func (c *APICommand) outputMode(rawJSON bool) apiOutputMode {
	if rawJSON {
		return apiOutputHuman
	}
	if c.Meta == nil {
		return apiOutputMachine
	}
	if c.Meta.HumanOutput {
		return apiOutputHuman
	}
	if c.Meta.StdoutIsTTY {
		return apiOutputHuman
	}
	return apiOutputMachine
}

func (c *APICommand) emitOutput(text string) {
	c.Meta.UI.Output(text)
}

func (c *APICommand) emitError(text string) {
	if c.Meta == nil || !c.Meta.HumanOutput || !c.Meta.StderrIsTTY {
		c.Meta.UI.Error(text)
		return
	}
	c.Meta.UI.Error(c.styles().error.Render(text))
}

func (c *APICommand) renderJSON(body []byte, mode apiOutputMode) string {
	pretty := render.PrettyJSON(body)
	if mode == apiOutputMachine || c.Meta == nil || !c.Meta.HumanOutput || !c.Meta.StdoutIsTTY {
		return pretty
	}
	return c.styles().json.Render(pretty)
}

func (c *APICommand) styles() apiStyles {
	return apiStyles{
		header: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")),
		label:  lipgloss.NewStyle().Foreground(lipgloss.Color("110")).Bold(true),
		error:  lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Bold(true),
		json:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
	}
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
