// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package run

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestRunStart(t *testing.T) {
	t.Parallel()

	t.Run("dry run with workspace ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/workspaces/ws-abc123", r.URL.Path)
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{
						"name": "foobar",
					},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{
								"id": "org-resolved", "type": "organizations",
							},
						},
					},
				},
			})
		}))

		err := runStart(context.Background(), StartOpts{
			IO:        io,
			Profile:   profile.TestProfile(t),
			Workspace: "ws-abc123",
			DryRun:    true,
			APIClient: c,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "would create run for workspace ID ws-abc123")
	})

	t.Run("dry run with workspace name resolves ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/organizations/my-org/workspaces/my-workspace", r.URL.Path)
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "my-workspace"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{
								"id": "org-resolved", "type": "organizations",
							},
						},
					},
				},
			})
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "my-workspace",
			Organization: "my-org",
			DryRun:       true,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "would create run for workspace ID ws-resolved")
	})

	t.Run("workspace name resolution failure", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "no-such-ws",
			Organization: "my-org",
			DryRun:       false,
		}, CreateOpts{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolving workspace")
	})

	t.Run("successful start with workspace ID", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/workspaces/ws-abc123":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "foobar"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-new123", "type": "runs",
						"attributes": map[string]any{"status": "pending"},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:        io,
			APIClient: c,
			Profile:   profile.TestProfile(t),
			Workspace: "ws-abc123",
			DryRun:    false,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "run-new123")
		assert.Contains(t, io.Error.String(), "created")
	})

	t.Run("successful start with workspace name", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/organizations/my-org/workspaces/my-ws":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "my-ws"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-fromname", "type": "runs",
						"attributes": map[string]any{"status": "pending"},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "my-ws",
			Organization: "my-org",
			DryRun:       false,
		}, CreateOpts{})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "run-fromname")
	})

	t.Run("successful start with create options", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/organizations/my-org/workspaces/my-ws":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "my-ws"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				data := body["data"].(map[string]any)
				attrs := data["attributes"].(map[string]any)
				assert.Equal(t, true, attrs["debugging-mode"])
				assert.Equal(t, "test start", attrs["message"])
				assert.Equal(t, true, attrs["allow-empty-apply"])

				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-fromname", "type": "runs",
						"attributes": map[string]any{"status": "pending"},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:           io,
			APIClient:    c,
			Profile:      profile.TestProfile(t),
			Workspace:    "my-ws",
			Organization: "my-org",
			DryRun:       false,
		}, CreateOpts{
			DebuggingMode:   true,
			Message:         "test start",
			AllowEmptyApply: true,
		})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "run-fromname")
	})

	t.Run("plan-only run sets plan-only attribute", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/workspaces/ws-abc123":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "foobar"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				data := body["data"].(map[string]any)
				attrs := data["attributes"].(map[string]any)
				assert.Equal(t, true, attrs["plan-only"])

				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "run-planonly", "type": "runs",
						"attributes": map[string]any{"status": "pending"},
					},
				})
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:        io,
			APIClient: c,
			Profile:   profile.TestProfile(t),
			Workspace: "ws-abc123",
			DryRun:    false,
		}, CreateOpts{
			PlanOnly: true,
		})

		require.NoError(t, err)
		assert.Contains(t, io.Error.String(), "run-planonly")
	})

	t.Run("API error on run creation", func(t *testing.T) {
		t.Parallel()
		io := iostreams.Test()

		c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch route(r) {
			case "GET /api/v2/workspaces/ws-abc123":
				jsonapi(w, map[string]any{
					"data": map[string]any{
						"id": "ws-resolved", "type": "workspaces",
						"attributes": map[string]any{"name": "foobar"},
						"relationships": map[string]any{
							"organization": map[string]any{
								"data": map[string]any{
									"id": "org-resolved", "type": "organizations",
								},
							},
						},
					},
				})
			case "POST /api/v2/runs":
				http.Error(w, "server error", http.StatusInternalServerError)
			default:
				http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
			}
		}))

		err := runStart(context.Background(), StartOpts{
			IO:        io,
			APIClient: c,
			Profile:   profile.TestProfile(t),
			Workspace: "ws-abc123",
			DryRun:    false,
		}, CreateOpts{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start run")
	})
}

func TestRunStart_Wait_Success(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	var runGets int32
	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-waited", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-waited":
			status := "planning"
			if atomic.AddInt32(&runGets, 1) > 1 {
				status = "planned_and_finished"
			}
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-waited", "type": "runs",
					"attributes": map[string]any{"status": status},
					"relationships": map[string]any{
						"workspace": map[string]any{
							"data": map[string]any{"id": "ws-abc123", "type": "workspaces"},
						},
					},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
	}, CreateOpts{})

	require.NoError(t, err)
	assert.Contains(t, io.Error.String(), "waiting for it to finish")
	assert.Contains(t, io.Error.String(), "planned_and_finished")
	// The final run summary is rendered to stdout, same as `run status`.
	assert.Contains(t, io.Output.String(), "Plan complete, no apply needed")
	// The run URL is surfaced in the displayer output.
	assert.Contains(t, io.Output.String(), "View run:")
	assert.Contains(t, io.Output.String(), "workspaces/foobar/runs/run-waited")
}

func TestRunStart_Wait_Quiet(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()
	io.SetQuiet(true)

	var runGets int32
	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-quiet", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-quiet":
			status := "planning"
			if atomic.AddInt32(&runGets, 1) > 1 {
				status = "planned_and_finished"
			}
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-quiet", "type": "runs",
					"attributes": map[string]any{"status": status},
					"relationships": map[string]any{
						"workspace": map[string]any{
							"data": map[string]any{"id": "ws-abc123", "type": "workspaces"},
						},
					},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
	}, CreateOpts{})

	require.NoError(t, err)
	// --quiet suppresses all wait progress on stderr...
	assert.Empty(t, io.Error.String())
	assert.NotContains(t, io.Error.String(), "waiting for it to finish")
	// ...but the final summary result still prints to stdout.
	assert.Contains(t, io.Output.String(), "Plan complete, no apply needed")
}

func TestRunStart_Wait_Timeout(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	// The run never settles; --wait must give up and still surface the URL.
	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-slow", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-slow":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-slow", "type": "runs",
					"attributes": map[string]any{"status": "planning"},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
		Timeout:      10 * time.Millisecond,
	}, CreateOpts{})

	require.Error(t, err)
	// The error chain includes either the context deadline or the cause message.
	errStr := err.Error()
	assert.True(t,
		strings.Contains(errStr, "--wait timeout exceeded") || strings.Contains(errStr, "deadline exceeded"),
		"expected timeout-related error, got: %s", errStr)
	assert.Contains(t, io.Error.String(), "still be running")
	assert.Contains(t, io.Error.String(), "workspaces/foobar/runs/run-slow")
}

func TestRunStart_Wait_AwaitingConfirm(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-confirm", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-confirm":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-confirm", "type": "runs",
					"attributes": map[string]any{
						"status":  "planned",
						"actions": map[string]any{"is-confirmable": true},
					},
					"relationships": map[string]any{
						"workspace": map[string]any{
							"data": map[string]any{"id": "ws-abc123", "type": "workspaces"},
						},
					},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
	}, CreateOpts{})

	require.NoError(t, err)
	// The generic "Run status: planned" line is replaced with actionable text,
	// and the apply URL is surfaced in the displayer output.
	assert.Contains(t, io.Output.String(), "manual apply is required")
	assert.NotContains(t, io.Output.String(), "Run status: planned")
	assert.Contains(t, io.Output.String(), "Confirm the apply by")
	assert.Contains(t, io.Output.String(), "workspaces/foobar/runs/run-confirm")
}

func TestRunStart_Wait_Failure(t *testing.T) {
	t.Parallel()
	io := iostreams.Test()

	// canceled is a failure state that NewRunSummary renders without any extra
	// API calls, so it exercises the full wait-then-exit-code path cleanly.
	c := testAPI(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch route(r) {
		case "GET /api/v2/workspaces/ws-abc123":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "ws-resolved", "type": "workspaces",
					"attributes": map[string]any{"name": "foobar"},
					"relationships": map[string]any{
						"organization": map[string]any{
							"data": map[string]any{"id": "my-org", "type": "organizations"},
						},
					},
				},
			})
		case "POST /api/v2/runs":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-cancel", "type": "runs",
					"attributes": map[string]any{"status": "pending"},
				},
			})
		case "GET /api/v2/runs/run-cancel":
			jsonapi(w, map[string]any{
				"data": map[string]any{
					"id": "run-cancel", "type": "runs",
					"attributes": map[string]any{"status": "canceled"},
					"relationships": map[string]any{
						"workspace": map[string]any{
							"data": map[string]any{"id": "ws-abc123", "type": "workspaces"},
						},
					},
				},
			})
		default:
			http.Error(w, "unexpected: "+route(r), http.StatusInternalServerError)
		}
	}))

	err := runStart(context.Background(), StartOpts{
		IO:           io,
		APIClient:    c,
		Profile:      profile.TestProfile(t),
		Output:       format.New(io),
		Workspace:    "ws-abc123",
		Wait:         true,
		PollInterval: time.Millisecond,
	}, CreateOpts{})

	require.ErrorIs(t, err, cmd.ErrUnderlyingError)
	assert.Contains(t, io.Output.String(), "Run was canceled")
	assert.Contains(t, io.Output.String(), "View run:")
	assert.Contains(t, io.Output.String(), "workspaces/foobar/runs/run-cancel")
}
