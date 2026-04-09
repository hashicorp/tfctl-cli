package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	cli "github.com/hashicorp/cli"

	"github.com/hashicorp/tfcloud/internal/client"
	"github.com/hashicorp/tfcloud/internal/config"
)

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("returns a single jsonapi resource and renders a vertical table", func(t *testing.T) {
		cmd, ui := newTestAPICommand(t, false, []responseStub{{statusCode: 200, body: `{"data":{"id":"run-1","type":"runs","attributes":{"message":"deploy","status":"planned_and_finished"}}}`}}, nil)

		if code := cmd.Run([]string{"/runs/run-1"}); code != 0 {
			t.Fatalf("got exit code %d: %s", code, ui.ErrorWriter.String())
		}
		got := ui.OutputWriter.String()
		if !strings.Contains(got, "message") || !strings.Contains(got, "deploy") {
			t.Fatalf("unexpected output %q", got)
		}
	})

	t.Run("returns a collection and renders a horizontal table", func(t *testing.T) {
		cmd, ui := newTestAPICommand(t, false, []responseStub{{statusCode: 200, body: `{"data":[{"id":"ws-1","type":"workspaces","attributes":{"name":"alpha","description":"one"}},{"id":"ws-2","type":"workspaces","attributes":{"name":"beta"}}]}`}}, nil)

		if code := cmd.Run([]string{"/workspaces"}); code != 0 {
			t.Fatalf("got exit code %d: %s", code, ui.ErrorWriter.String())
		}
		got := ui.OutputWriter.String()
		if !strings.Contains(got, "alpha") || !strings.Contains(got, "workspaces") {
			t.Fatalf("unexpected output %q", got)
		}
	})

	t.Run("prints a useful error message when the api returns an error response", func(t *testing.T) {
		cmd, ui := newTestAPICommand(t, false, []responseStub{{statusCode: 422, status: "422 Unprocessable Entity", body: `{"errors":[{"title":"invalid attribute","detail":"name is required"}]}`}}, nil)

		if code := cmd.Run([]string{"/projects"}); code != 1 {
			t.Fatalf("got exit code %d", code)
		}
		got := ui.ErrorWriter.String()
		if !strings.Contains(got, "422 Unprocessable Entity") || !strings.Contains(got, "name is required") {
			t.Fatalf("unexpected error output %q", got)
		}
	})

	t.Run("reads request body from stdin with -i dash", func(t *testing.T) {
		var captured []byte
		cmd, ui := newTestAPICommand(t, false, []responseStub{{statusCode: 200, body: `{"data":[]}`}}, func(req *client.Request) {
			captured = append([]byte(nil), req.Body...)
		})
		cmd.Meta.Stdin = strings.NewReader(`{"data":{"type":"vars"}}`)

		if code := cmd.Run([]string{"/vars", "-i", "-"}); code != 0 {
			t.Fatalf("got exit code %d: %s", code, ui.ErrorWriter.String())
		}
		if string(captured) != `{"data":{"type":"vars"}}` {
			t.Fatalf("got body %q", captured)
		}
	})

	t.Run("supports input file, headers, fields, type, method, silent, verbose, and json alias flags", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("X-Test"); got != "yes" {
				t.Fatalf("got header %q", got)
			}
			if r.URL.RawQuery != "page%5Bnumber%5D=2" {
				t.Fatalf("got query %q", r.URL.RawQuery)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if len(body) == 0 {
				t.Fatal("expected request body")
			}
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		defer server.Close()

		inputFile := t.TempDir() + "/input.json"
		if err := os.WriteFile(inputFile, []byte(`{"data":{"type":"vars","attributes":{"key":"AWS_REGION"}}}`), 0o600); err != nil {
			t.Fatal(err)
		}

		cmd, ui := newTestAPICommandWithServer(t, false, server)

		if code := cmd.Run([]string{"/vars", "-i", inputFile, "-X", "post", "-H", "X-Test: yes", "-f", "page[number]=2", "-v", "-silent"}); code != 0 {
			t.Fatalf("got exit code %d: %s", code, ui.ErrorWriter.String())
		}
		if ui.OutputWriter.String() != "" {
			t.Fatalf("expected no output, got %q", ui.OutputWriter.String())
		}
		if got := ui.ErrorWriter.String(); !strings.Contains(got, "> POST") || !strings.Contains(got, "< 200 OK") {
			t.Fatalf("unexpected verbose output %q", got)
		}
	})

	t.Run("builds typed jsonapi body with attributes and explicit type", func(t *testing.T) {
		var captured []byte
		cmd, ui := newTestAPICommand(t, false, []responseStub{{statusCode: 200, body: `{"data":[]}`}}, func(req *client.Request) {
			captured = append([]byte(nil), req.Body...)
		})

		if code := cmd.Run([]string{"/vars", "-t", "vars", "-a", "key=AWS_REGION", "-a", "hcl=false"}); code != 0 {
			t.Fatalf("got exit code %d: %s", code, ui.ErrorWriter.String())
		}

		var payload map[string]any
		if err := json.Unmarshal(captured, &payload); err != nil {
			t.Fatal(err)
		}
		data := payload["data"].(map[string]any)
		if data["type"] != "vars" {
			t.Fatalf("got type %v", data["type"])
		}
	})

	t.Run("paginates and merges resources", func(t *testing.T) {
		var serverURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			switch r.URL.Path {
			case "/api/v2/workspaces":
				_, _ = fmt.Fprintf(w, `{"data":[{"id":"ws-1","type":"workspaces","attributes":{"name":"alpha"}}],"links":{"next":%q},"meta":{"pagination":{"total-count":1}}}`, serverURL+"/api/v2/page/2")
			case "/api/v2/page/2":
				_, _ = w.Write([]byte(`{"data":[{"id":"ws-2","type":"workspaces","attributes":{"name":"beta"}}],"links":{"next":null}}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()
		serverURL = server.URL

		cmd, ui := newTestAPICommandWithServer(t, false, server)
		if code := cmd.Run([]string{"/workspaces", "-paginate"}); code != 0 {
			t.Fatalf("got exit code %d: %s", code, ui.ErrorWriter.String())
		}
		got := ui.OutputWriter.String()
		if !strings.Contains(got, "alpha") || !strings.Contains(got, "beta") {
			t.Fatalf("unexpected output %q", got)
		}
	})

	t.Run("when output is piped to a file the output is raw json instead of a table", func(t *testing.T) {
		cmd, ui := newTestAPICommand(t, true, []responseStub{{statusCode: 200, body: `{"data":[{"id":"ws-1","type":"workspaces","attributes":{"name":"alpha"}}]}`}}, nil)

		if code := cmd.Run([]string{"/workspaces"}); code != 0 {
			t.Fatalf("got exit code %d", code)
		}
		if got := strings.TrimSpace(ui.OutputWriter.String()); !strings.HasPrefix(got, "{") {
			t.Fatalf("expected json output, got %q", got)
		}
	})

	t.Run("agent is an alias for json", func(t *testing.T) {
		cmd, ui := newTestAPICommand(t, false, []responseStub{{statusCode: 200, body: `{"data":[{"id":"ws-1","type":"workspaces","attributes":{"name":"alpha"}}]}`}}, nil)

		if code := cmd.Run([]string{"/workspaces", "-agent"}); code != 0 {
			t.Fatalf("got exit code %d", code)
		}
		if got := strings.TrimSpace(ui.OutputWriter.String()); !strings.HasPrefix(got, "{") {
			t.Fatalf("expected json output, got %q", got)
		}
	})
}

func TestAPIHelpStylesSectionHeaders(t *testing.T) {
	t.Parallel()

	help := (&APICommand{}).Help()
	for _, section := range []string{"Options:", "Path templates:", "Examples:"} {
		styled := "\x1b[1;97m" + section + "\x1b[0m"
		if !strings.Contains(help, styled) {
			t.Fatalf("missing section %q in help %q", section, help)
		}
	}
}

func TestInferMethod(t *testing.T) {
	t.Parallel()

	if got := inferMethod("", false, false); got != "GET" {
		t.Fatalf("got %q", got)
	}
	if got := inferMethod("", true, false); got != "POST" {
		t.Fatalf("got %q", got)
	}
	if got := inferMethod("patch", false, false); got != "PATCH" {
		t.Fatalf("got %q", got)
	}
}

func TestInferResourceType(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"/organizations/acme/projects":        "projects",
		"/runs":                               "runs",
		"/projects/prj-123/tag-bindings":      "tag-bindings",
		"/organizations/acme/workspaces/ws-1": "workspaces",
		"/workspaces/ws-1/vars":               "vars",
		"/varsets/vs-1/relationships/vars":    "vars",
	}

	for input, want := range tests {
		if got := inferResourceType(input); got != want {
			t.Fatalf("inferResourceType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLooksLikeCollection(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"projects":      true,
		"runs":          true,
		"workspaces":    true,
		"vars":          true,
		"varsets":       true,
		"policy-sets":   true,
		"organizations": true,
		"ws-1":          false,
		"relationships": true,
		"tag-bindings":  true,
	}

	for input, want := range tests {
		if got := looksLikeCollection(input); got != want {
			t.Fatalf("looksLikeCollection(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestSplitPair(t *testing.T) {
	t.Parallel()

	if gotKey, gotValue, err := splitPair("foo=bar", '='); err != nil || gotKey != "foo" || gotValue != "bar" {
		t.Fatalf("got %q %q %v", gotKey, gotValue, err)
	}

	if _, _, err := splitPair("missing", '='); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseTypedValue(t *testing.T) {
	t.Parallel()

	if got := parseTypedValue("true"); got != true {
		t.Fatalf("got %#v", got)
	}
	if got := parseTypedValue("42"); got != int64(42) {
		t.Fatalf("got %#v", got)
	}
	if got := parseTypedValue("3.14"); got != 3.14 {
		t.Fatalf("got %#v", got)
	}
	if got := parseTypedValue("hello"); got != "hello" {
		t.Fatalf("got %#v", got)
	}
}

func TestMergePaginatedBody(t *testing.T) {
	t.Parallel()

	body := []byte(`{"data":[{"id":"1"}],"links":{"next":"https://example.com/page/2"},"meta":{"pagination":{"total-count":1}}}`)
	merged, err := mergePaginatedBody(body, []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}})
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(merged, &payload); err != nil {
		t.Fatal(err)
	}
	data := payload["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("got %d rows", len(data))
	}
	if payload["links"].(map[string]any)["next"] != nil {
		t.Fatal("expected next link cleared")
	}
}

func TestBuildRequestBodyInfersTypedJSONAPI(t *testing.T) {
	t.Parallel()

	body, contentType, err := buildRequestBody("/workspaces/ws-1/vars", "", multiFlag{"enabled=true", "count=42", "config={\"mode\":\"safe\"}"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "application/vnd.api+json" {
		t.Fatalf("got content type %q", contentType)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	data := payload["data"].(map[string]any)
	if data["type"] != "vars" {
		t.Fatalf("got type %v", data["type"])
	}
	attrs := data["attributes"].(map[string]any)
	if got, want := attrs["enabled"], true; !reflect.DeepEqual(got, want) {
		t.Fatalf("enabled = %#v, want %#v", got, want)
	}
}

type responseStub struct {
	statusCode int
	status     string
	body       string
	headers    http.Header
}

type stubAPIClient struct {
	baseURL        *url.URL
	responses      []responseStub
	requestHandler func(*client.Request)
	index          int
}

type serverAPIClient struct {
	httpClient *http.Client
	baseURL    *url.URL
}

func (c *stubAPIClient) Base() *url.URL {
	return c.baseURL
}

func (c *stubAPIClient) RawRequest(_ context.Context, req *client.Request) (*client.Response, error) {
	if c.requestHandler != nil {
		c.requestHandler(req)
	}
	if c.index >= len(c.responses) {
		return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL)
	}
	resp := c.responses[c.index]
	c.index++
	status := resp.status
	if status == "" {
		status = strconv.Itoa(resp.statusCode) + " " + http.StatusText(resp.statusCode)
	}
	headers := resp.headers
	if headers == nil {
		headers = make(http.Header)
	}
	return &client.Response{StatusCode: resp.statusCode, Status: status, Headers: headers, Body: []byte(resp.body)}, nil
}

func (c *serverAPIClient) Base() *url.URL {
	return c.baseURL
}

func (c *serverAPIClient) RawRequest(ctx context.Context, req *client.Request) (*client.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL.String(), bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for key, values := range req.Headers {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	return &client.Response{StatusCode: httpResp.StatusCode, Status: httpResp.Status, Headers: httpResp.Header.Clone(), Body: body}, nil
}

func newTestAPICommand(t *testing.T, machine bool, responses []responseStub, handler func(*client.Request)) (*APICommand, *cli.MockUi) {
	t.Helper()
	baseURL, err := url.Parse("https://app.terraform.test/api/v2")
	if err != nil {
		t.Fatal(err)
	}
	ui := cli.NewMockUi()
	meta := &Meta{
		UI:          ui,
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      ui.OutputWriter,
		Stderr:      ui.ErrorWriter,
		StdoutIsTTY: !machine,
		StderrIsTTY: !machine,
		HumanOutput: !machine,
	}
	cmd := &APICommand{
		Meta: meta,
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Hostname: "app.terraform.test", Token: "token", DefaultHeaders: make(http.Header)}, nil
		},
		newClient: func(*config.Config) (apiRequester, error) {
			return &stubAPIClient{baseURL: baseURL, responses: responses, requestHandler: handler}, nil
		},
	}
	return cmd, ui
}

func newTestAPICommandWithServer(t *testing.T, machine bool, server *httptest.Server) (*APICommand, *cli.MockUi) {
	t.Helper()
	baseURL, err := url.Parse(server.URL + "/api/v2")
	if err != nil {
		t.Fatal(err)
	}
	ui := cli.NewMockUi()
	meta := &Meta{
		UI:          ui,
		Stdin:       bytes.NewBuffer(nil),
		Stdout:      ui.OutputWriter,
		Stderr:      ui.ErrorWriter,
		StdoutIsTTY: !machine,
		StderrIsTTY: !machine,
		HumanOutput: !machine,
	}
	cmd := &APICommand{
		Meta: meta,
		loadConfig: func() (*config.Config, error) {
			return &config.Config{Hostname: baseURL.Hostname(), Token: "token", DefaultHeaders: make(http.Header)}, nil
		},
		newClient: func(cfg *config.Config) (apiRequester, error) {
			return &serverAPIClient{httpClient: server.Client(), baseURL: baseURL}, nil
		},
	}
	return cmd, ui
}
