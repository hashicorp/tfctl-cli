// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package module

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	tfe "github.com/hashicorp/go-tfe/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdtest"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

const publishPath = "/api/v2/organizations/my-org/registry-modules/vcs"

func TestNewCmdPublishArguments(t *testing.T) {
	t.Parallel()

	t.Run("accepts no positional arguments", func(t *testing.T) {
		t.Parallel()

		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{
			"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
				writePublishJSONAPI(w, http.StatusCreated, publishResponse("/api/v2/registry-modules/mod-123", "setup_complete"))
			},
		}))
		inv.Profile.DefaultOrganization = "my-org"

		publish := NewCmdPublish(inv)
		root := &cmd.Command{Name: "tfctl"}
		module := &cmd.Command{Name: "module"}
		module.AddChild(publish)
		root.AddChild(module)
		cmd.ConfigureRootCommand(inv, root)

		exitCode := publish.Run([]string{
			"--repo", "acme/terraform-aws-network",
			"--oauth-token-id", "ot-valid",
		}, inv)
		assert.Equal(t, 0, exitCode)
	})

	t.Run("rejects a positional argument", func(t *testing.T) {
		t.Parallel()

		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))
		publish := NewCmdPublish(inv)
		root := &cmd.Command{Name: "tfctl"}
		module := &cmd.Command{Name: "module"}
		module.AddChild(publish)
		root.AddChild(module)
		cmd.ConfigureRootCommand(inv, root)

		exitCode := publish.Run([]string{
			"extra",
			"--repo", "acme/terraform-aws-network",
			"--oauth-token-id", "ot-valid",
		}, inv)
		assert.NotEqual(t, 0, exitCode)
		assert.Contains(t, io.Error.String(), "no arguments allowed")
	})

	t.Run("requires repository flag", func(t *testing.T) {
		t.Parallel()

		io := iostreams.Test()
		inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))
		publish := NewCmdPublish(inv)
		root := &cmd.Command{Name: "tfctl"}
		module := &cmd.Command{Name: "module"}
		module.AddChild(publish)
		root.AddChild(module)
		cmd.ConfigureRootCommand(inv, root)

		exitCode := publish.Run([]string{"--oauth-token-id", "ot-valid"}, inv)
		assert.NotEqual(t, 0, exitCode)
		assert.Contains(t, io.Error.String(), "missing required flag: --repo")
	})
}

func TestNewCmdPublishHelpDocumentsConnectionAndRepositoryLimits(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	inv := cmdtest.NewInvocation(t, io, cmdtest.NewServer(t, cmdtest.RouteMap{}))
	publish := NewCmdPublish(inv)
	longHelp := strings.Join(strings.Fields(publish.LongHelp), " ")

	assert.Contains(t, longHelp, "exactly one of --oauth-token-id or --github-app-installation-id")
	assert.Contains(t, longHelp, "tfctl api /organizations/{organization}/oauth-tokens --all")
	assert.Contains(t, longHelp, "Account Settings")
	assert.Contains(t, longHelp, "such as some Bitbucket Cloud repositories")
	assert.Contains(t, longHelp, "Use tfctl api for those repositories.")
}

func TestNewCmdPublishOptionalFlagPresence(t *testing.T) {
	t.Parallel()

	const profilePublishPath = "/api/v2/organizations/profile-org/registry-modules/vcs"
	baseAttributes := func(vcsRepo map[string]any) map[string]any {
		return map[string]any{
			"data": map[string]any{
				"type": "registry-modules",
				"attributes": map[string]any{
					"vcs-repo": vcsRepo,
				},
			},
		}
	}

	tests := map[string]struct {
		args     []string
		wantErr  string
		wantBody map[string]any
	}{
		"explicit empty organization is rejected": {
			args: []string{
				"--repo", "acme/terraform-aws-network",
				"--oauth-token-id", "ot-valid",
				"--organization=",
			},
			wantErr: "--organization must not be blank",
		},
		"explicit empty branch is rejected": {
			args: []string{
				"--repo", "acme/terraform-aws-network",
				"--oauth-token-id", "ot-valid",
				"--branch=",
			},
			wantErr: "--branch must not be blank",
		},
		"explicit empty initial version is rejected": {
			args: []string{
				"--repo", "acme/terraform-aws-network",
				"--oauth-token-id", "ot-valid",
				"--branch", "main",
				"--initial-version=",
			},
			wantErr: "--initial-version must not be blank",
		},
		"omitted organization uses profile fallback": {
			args: []string{
				"--repo", "acme/terraform-aws-network",
				"--oauth-token-id", "ot-valid",
				"--branch", "main",
				"--initial-version", "1.2.3",
			},
			wantBody: map[string]any{
				"data": map[string]any{
					"type": "registry-modules",
					"attributes": map[string]any{
						"initial-version": "1.2.3",
						"vcs-repo": map[string]any{
							"identifier":         "acme/terraform-aws-network",
							"display-identifier": "acme/terraform-aws-network",
							"oauth-token-id":     "ot-valid",
							"branch":             "main",
						},
					},
				},
			},
		},
		"omitted branch selects tag publishing": {
			args: []string{
				"--repo", "acme/terraform-aws-network",
				"--oauth-token-id", "ot-valid",
			},
			wantBody: baseAttributes(map[string]any{
				"identifier":         "acme/terraform-aws-network",
				"display-identifier": "acme/terraform-aws-network",
				"oauth-token-id":     "ot-valid",
			}),
		},
		"omitted initial version is valid": {
			args: []string{
				"--repo", "acme/terraform-aws-network",
				"--oauth-token-id", "ot-valid",
				"--branch", "main",
			},
			wantBody: baseAttributes(map[string]any{
				"identifier":         "acme/terraform-aws-network",
				"display-identifier": "acme/terraform-aws-network",
				"oauth-token-id":     "ot-valid",
				"branch":             "main",
			}),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var requestCount atomic.Int32
			gotRequest := make(chan capturedRequest, 1)
			streams := iostreams.Test()
			inv := cmdtest.NewInvocation(t, streams, cmdtest.NewServer(t, cmdtest.RouteMap{
				"POST " + profilePublishPath: func(w http.ResponseWriter, r *http.Request) {
					requestCount.Add(1)
					body, err := io.ReadAll(r.Body)
					gotRequest <- capturedRequest{body: body, err: err}
					writePublishJSONAPI(w, http.StatusCreated, publishResponse("/api/v2/registry-modules/mod-123", "setup_complete"))
				},
			}))
			inv.Profile.DefaultOrganization = "profile-org"

			publish := NewCmdPublish(inv)
			root := &cmd.Command{Name: "tfctl"}
			module := &cmd.Command{Name: "module"}
			module.AddChild(publish)
			root.AddChild(module)
			cmd.ConfigureRootCommand(inv, root)

			exitCode := publish.Run(tc.args, inv)
			if tc.wantErr != "" {
				assert.NotEqual(t, 0, exitCode)
				assert.Contains(t, streams.Error.String(), tc.wantErr)
				assert.EqualValues(t, 0, requestCount.Load())
				return
			}

			require.Equal(t, 0, exitCode)
			require.EqualValues(t, 1, requestCount.Load())
			request := <-gotRequest
			require.NoError(t, request.err)
			want, err := json.Marshal(tc.wantBody)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(request.body))
		})
	}
}

func TestRunPublishValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		mutate  func(*PublishOpts)
		wantErr string
	}{
		"repository is required": {
			mutate: func(opts *PublishOpts) {
				opts.Repository = "  "
			},
			wantErr: "repository is required",
		},
		"a connection is required": {
			mutate: func(opts *PublishOpts) {
				opts.OAuthTokenID = ""
			},
			wantErr: "exactly one of --oauth-token-id or --github-app-installation-id",
		},
		"connections are mutually exclusive": {
			mutate: func(opts *PublishOpts) {
				opts.GitHubAppInstallationID = "ghain-both"
			},
			wantErr: "exactly one of --oauth-token-id or --github-app-installation-id",
		},
		"initial version requires branch": {
			mutate: func(opts *PublishOpts) {
				opts.InitialVersion = publishStringPointer("1.2.3")
			},
			wantErr: "--initial-version requires --branch",
		},
		"explicit organization must not be blank": {
			mutate: func(opts *PublishOpts) {
				opts.ProfileOrganization = "profile-org"
				opts.Organization = publishStringPointer(" \t ")
			},
			wantErr: "--organization must not be blank",
		},
		"branch must not be blank": {
			mutate: func(opts *PublishOpts) {
				opts.Branch = publishStringPointer(" \t ")
			},
			wantErr: "--branch must not be blank",
		},
		"initial version must not be blank": {
			mutate: func(opts *PublishOpts) {
				opts.Branch = publishStringPointer("main")
				opts.InitialVersion = publishStringPointer(" \t ")
			},
			wantErr: "--initial-version must not be blank",
		},
		"organization is required": {
			mutate: func(opts *PublishOpts) {
				opts.ProfileOrganization = ""
			},
			wantErr: "organization is required but not set",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, _ := newPublishTestOpts(t, cmdtest.RouteMap{})
			tc.mutate(opts)

			err := runPublish(context.Background(), opts)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestRunPublishRequestContract(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path   string
		mutate func(*PublishOpts)
		want   map[string]any
	}{
		"OAuth tag publishing uses profile organization": {
			path: "/api/v2/organizations/profile-org/registry-modules/vcs",
			mutate: func(opts *PublishOpts) {
				opts.ProfileOrganization = "profile-org"
				opts.Repository = "  acme/terraform-aws-network  "
				opts.OAuthTokenID = "ot-123"
			},
			want: map[string]any{
				"data": map[string]any{
					"type": "registry-modules",
					"attributes": map[string]any{
						"vcs-repo": map[string]any{
							"identifier":         "acme/terraform-aws-network",
							"display-identifier": "acme/terraform-aws-network",
							"oauth-token-id":     "ot-123",
						},
					},
				},
			},
		},
		"GitHub App branch publishing uses explicit organization": {
			path: "/api/v2/organizations/flag-org/registry-modules/vcs",
			mutate: func(opts *PublishOpts) {
				opts.ProfileOrganization = "profile-org"
				opts.Organization = publishStringPointer("flag-org")
				opts.Repository = "acme/terraform-google-network"
				opts.OAuthTokenID = ""
				opts.GitHubAppInstallationID = "ghain-456"
				opts.Branch = publishStringPointer("main")
				opts.InitialVersion = publishStringPointer("1.2.3")
			},
			want: map[string]any{
				"data": map[string]any{
					"type": "registry-modules",
					"attributes": map[string]any{
						"initial-version": "1.2.3",
						"vcs-repo": map[string]any{
							"identifier":                 "acme/terraform-google-network",
							"display-identifier":         "acme/terraform-google-network",
							"github-app-installation-id": "ghain-456",
							"branch":                     "main",
						},
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			gotRequest := make(chan capturedRequest, 1)
			opts, _ := newPublishTestOpts(t, cmdtest.RouteMap{
				"POST " + tc.path: func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					gotRequest <- capturedRequest{
						contentType: r.Header.Get("Content-Type"),
						body:        body,
						err:         err,
					}
					writePublishJSONAPI(w, http.StatusCreated, publishResponse("/api/v2/registry-modules/mod-123", "setup_complete"))
				},
			})
			tc.mutate(opts)

			err := runPublish(context.Background(), opts)
			require.NoError(t, err)

			request := <-gotRequest
			require.NoError(t, request.err)
			assert.Equal(t, "application/vnd.api+json", request.contentType)
			want, err := json.Marshal(tc.want)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(request.body))
		})
	}
}

func TestRunPublishDryRun(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	opts, streams := newPublishTestOpts(t, cmdtest.RouteMap{
		"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
			requestCount.Add(1)
			writePublishJSONAPI(w, http.StatusCreated, publishResponse("/api/v2/registry-modules/mod-123", "setup_complete"))
		},
	})
	opts.OAuthTokenID = ""
	opts.GitHubAppInstallationID = "ghain-do-not-print"
	opts.Branch = publishStringPointer("main")
	opts.DryRun = true

	err := runPublish(context.Background(), opts)
	require.NoError(t, err)
	assert.EqualValues(t, 0, requestCount.Load())
	assert.Empty(t, streams.Output.String())

	diagnostic := streams.Error.String()
	assert.Contains(t, diagnostic, "DRY RUN:")
	assert.Contains(t, diagnostic, "acme/terraform-aws-network")
	assert.Contains(t, diagnostic, "my-org")
	assert.Contains(t, strings.ToLower(diagnostic), "branch")
	assert.NotContains(t, diagnostic, "ghain-do-not-print")
	assert.NotContains(t, diagnostic, "github-app-installation-id")
	assert.NotContains(t, diagnostic, `"data"`)
}

func TestRunPublishOutputFormats(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		format       format.Format
		selfLink     string
		assertOutput func(*testing.T, string, string, string)
	}{
		"default": {
			format:   format.Unset,
			selfLink: "/api/v2/organizations/my-org/registry-modules/private/my-org/network/aws",
			assertOutput: func(t *testing.T, output, resolvedSelf, htmlLink string) {
				t.Helper()
				assert.Contains(t, output, "mod-123")
				assert.Contains(t, output, "network")
				assert.Contains(t, output, "my-org")
				assert.Contains(t, output, "aws")
				assert.Contains(t, output, "setup_complete")
				assert.Contains(t, output, resolvedSelf)
				assert.Contains(t, output, htmlLink)
			},
		},
		"JSON": {
			format:   format.JSON,
			selfLink: "https://app.example.test/api/v2/organizations/my-org/registry-modules/private/my-org/network/aws",
			assertOutput: func(t *testing.T, output, resolvedSelf, htmlLink string) {
				t.Helper()
				var got map[string]any
				require.NoError(t, json.Unmarshal([]byte(output), &got))
				assert.Equal(t, map[string]any{
					"id":        "mod-123",
					"name":      "network",
					"namespace": "my-org",
					"provider":  "aws",
					"status":    "setup_complete",
					"self_link": resolvedSelf,
					"html_link": htmlLink,
				}, got)
			},
		},
		"Markdown": {
			format:   format.Markdown,
			selfLink: "/api/v2/organizations/my-org/registry-modules/private/my-org/network/aws",
			assertOutput: func(t *testing.T, output, resolvedSelf, htmlLink string) {
				t.Helper()
				assert.Contains(t, output, "| Field")
				assert.Contains(t, output, "| ID")
				assert.Contains(t, output, "mod-123")
				assert.Contains(t, output, "network")
				assert.Contains(t, output, "my-org")
				assert.Contains(t, output, "aws")
				assert.Contains(t, output, "setup_complete")
				assert.Contains(t, output, resolvedSelf)
				assert.Contains(t, output, htmlLink)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts, streams := newPublishTestOpts(t, cmdtest.RouteMap{
				"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
					writePublishJSONAPI(w, http.StatusCreated, publishResponse(tc.selfLink, "setup_complete"))
				},
			})
			if tc.format != format.Unset {
				opts.Output.SetFormat(tc.format)
			}

			err := runPublish(context.Background(), opts)
			require.NoError(t, err)

			resolvedSelf := tc.selfLink
			if strings.HasPrefix(tc.selfLink, "/") {
				resolvedSelf = strings.TrimSuffix(opts.Client.BaseURL.Scheme+"://"+opts.Client.BaseURL.Host, "/") + tc.selfLink
			}
			htmlLink := opts.Client.BaseURL.Scheme + "://" + opts.Client.BaseURL.Host + "/app/my-org/registry/modules/private/my-org/network/aws"
			output := streams.Output.String()
			tc.assertOutput(t, output, resolvedSelf, htmlLink)

			for _, unsafe := range []string{
				"ot-response-secret",
				"ghain-response-secret",
				"webhook-response-secret",
				"vcs-repo",
				"relationships",
			} {
				assert.NotContains(t, output, unsafe)
			}
		})
	}
}

func TestRunPublishPendingAndQuiet(t *testing.T) {
	t.Parallel()

	t.Run("pending response returns without polling", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		opts, streams := newPublishTestOpts(t, cmdtest.RouteMap{
			"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
				requestCount.Add(1)
				writePublishJSONAPI(w, http.StatusCreated, publishResponse("/api/v2/registry-modules/mod-pending", "pending"))
			},
		})

		err := runPublish(context.Background(), opts)
		require.NoError(t, err)
		assert.EqualValues(t, 1, requestCount.Load())
		assert.Contains(t, streams.Output.String(), "pending")
		diagnostic := strings.ToLower(streams.Error.String())
		assert.Contains(t, diagnostic, "accepted")
		assert.Contains(t, diagnostic, "processing")
	})

	t.Run("quiet suppresses successful output and guidance", func(t *testing.T) {
		t.Parallel()

		var requestCount atomic.Int32
		opts, streams := newPublishTestOpts(t, cmdtest.RouteMap{
			"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
				requestCount.Add(1)
				writePublishJSONAPI(w, http.StatusCreated, publishResponse("/api/v2/registry-modules/mod-pending", "pending"))
			},
		})
		opts.Quiet = true
		streams.SetQuiet(true)

		err := runPublish(context.Background(), opts)
		require.NoError(t, err)
		assert.EqualValues(t, 1, requestCount.Load())
		assert.Empty(t, streams.Output.String())
		assert.Empty(t, streams.Error.String())
	})
}

func TestRunPublishAPIValidationErrorPropagatesInQuietMode(t *testing.T) {
	t.Parallel()

	opts, streams := newPublishTestOpts(t, cmdtest.RouteMap{
		"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
			writePublishJSONAPI(w, http.StatusUnprocessableEntity, map[string]any{
				"errors": []any{
					map[string]any{
						"title":  "Validation failed",
						"detail": "repository identifier is invalid",
					},
				},
			})
		},
	})
	opts.Quiet = true
	streams.SetQuiet(true)

	err := runPublish(context.Background(), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, tfe.ErrUnprocessableEntity)
	var apiErr *tfe.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, []string{"Validation failed: repository identifier is invalid"}, apiErr.Details)
}

func TestRunPublishSelfLinkResolutionError(t *testing.T) {
	t.Parallel()

	opts, streams := newPublishTestOpts(t, cmdtest.RouteMap{
		"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
			writePublishJSONAPI(w, http.StatusCreated, publishResponse("%", "setup_complete"))
		},
	})

	err := runPublish(context.Background(), opts)
	require.ErrorContains(t, err, "failed to resolve registry module self link")
	assert.ErrorContains(t, err, "invalid URL escape")
	assert.Empty(t, streams.Output.String())
}

func TestRunPublishMalformedSuccess(t *testing.T) {
	t.Parallel()

	opts, _ := newPublishTestOpts(t, cmdtest.RouteMap{
		"POST " + publishPath: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"data":`)
		},
	})

	err := runPublish(context.Background(), opts)
	require.ErrorContains(t, err, "decode")
	var syntaxErr *json.SyntaxError
	require.ErrorAs(t, err, &syntaxErr)
}

func TestResolvePublishSelfLink(t *testing.T) {
	t.Parallel()

	base := &url.URL{Scheme: "https", Host: "app.example.test", Path: "/api/v2/"}
	tests := map[string]struct {
		base    *url.URL
		self    string
		want    string
		wantErr string
	}{
		"absolute link": {
			base: base,
			self: "https://modules.example.test/api/v2/registry-modules/mod-123",
			want: "https://modules.example.test/api/v2/registry-modules/mod-123",
		},
		"root-relative link": {
			base: base,
			self: "/api/v2/registry-modules/mod-123",
			want: "https://app.example.test/api/v2/registry-modules/mod-123",
		},
		"network-path link keeps configured origin": {
			base: base,
			self: "//modules.example.test/api/v2/registry-modules/mod-123",
			want: "https://app.example.test/api/v2/registry-modules/mod-123",
		},
		"malformed link": {
			base:    base,
			self:    "%",
			wantErr: "invalid URL escape",
		},
		"relative link requires base": {
			self:    "/api/v2/registry-modules/mod-123",
			wantErr: "configured API origin is missing",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := resolvePublishSelfLink(tc.base, tc.self)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolvePublishHTMLLink(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		base         *url.URL
		organization string
		name         string
		provider     string
		want         string
	}{
		"HCP Terraform": {
			base:         &url.URL{Scheme: "https", Host: "app.terraform.io", Path: "/api/v2/"},
			organization: "my-org",
			name:         "network",
			provider:     "aws",
			want:         "https://app.terraform.io/app/my-org/registry/modules/private/my-org/network/aws",
		},
		"Terraform Enterprise": {
			base:         &url.URL{Scheme: "https", Host: "tfe.example.test", Path: "/api/v2/"},
			organization: "my-org",
			name:         "network",
			provider:     "aws",
			want:         "https://tfe.example.test/app/my-org/registry/modules/private/my-org/network/aws",
		},
		"escapes path segments": {
			base:         &url.URL{Scheme: "https", Host: "app.terraform.io", Path: "/api/v2/"},
			organization: "my org",
			name:         "network/core",
			provider:     "aws cloud",
			want:         "https://app.terraform.io/app/my%20org/registry/modules/private/my%20org/network%2Fcore/aws%20cloud",
		},
		"missing required data": {
			base:         &url.URL{Scheme: "https", Host: "app.terraform.io", Path: "/api/v2/"},
			organization: "my-org",
			provider:     "aws",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := resolvePublishHTMLLink(tc.base, tc.organization, tc.name, tc.provider)
			assert.Equal(t, tc.want, got)
		})
	}
}

type capturedRequest struct {
	contentType string
	body        []byte
	err         error
}

func newPublishTestOpts(t *testing.T, routes cmdtest.RouteMap) (*PublishOpts, *iostreams.Testing) {
	t.Helper()

	streams := iostreams.Test()
	inv := cmdtest.NewInvocation(t, streams, cmdtest.NewServer(t, routes))
	apiClient, err := inv.NewAPIClient()
	require.NoError(t, err)

	return &PublishOpts{
		IO:                  streams,
		Output:              inv.Output,
		Client:              apiClient,
		ProfileOrganization: "my-org",
		Repository:          "acme/terraform-aws-network",
		OAuthTokenID:        "ot-valid",
	}, streams
}

func publishResponse(selfLink, status string) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"id":   "mod-123",
			"type": "registry-modules",
			"attributes": map[string]any{
				"name":      "network",
				"namespace": "my-org",
				"provider":  "aws",
				"status":    status,
				"vcs-repo": map[string]any{
					"identifier":                 "acme/terraform-aws-network",
					"oauth-token-id":             "ot-response-secret",
					"github-app-installation-id": "ghain-response-secret",
					"webhook-url":                "webhook-response-secret",
				},
			},
			"relationships": map[string]any{
				"organization": map[string]any{
					"data": map[string]any{"id": "my-org", "type": "organizations"},
				},
			},
			"links": map[string]any{
				"self": selfLink,
			},
		},
	}
}

func writePublishJSONAPI(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	cmdtest.WriteJSONAPI(w, payload)
}

func publishStringPointer(value string) *string {
	return &value
}
