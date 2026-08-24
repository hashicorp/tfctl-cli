// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/commands/cmdutil"
	"github.com/hashicorp/tfctl-cli/internal/pkg/client"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/version"
)

const (
	registryModulePublishPath = "/organizations/{organization_name}/registry-modules/vcs"
	jsonAPIContentType        = "application/vnd.api+json"
)

// PublishOpts defines the options for the `module publish` command.
type PublishOpts struct {
	IO                      iostreams.IOStreams
	Output                  *format.Outputter
	Client                  *client.Client
	ProfileOrganization     string
	Organization            *string
	Repository              string
	OAuthTokenID            string
	GitHubAppInstallationID string
	Branch                  *string
	InitialVersion          *string
	DryRun                  bool
	Quiet                   bool
}

// NewCmdPublish creates the `module publish` command.
func NewCmdPublish(inv *cmd.Invocation) *cmd.Command {
	opts := &PublishOpts{
		IO:     inv.IO,
		Output: inv.Output,
	}

	return &cmd.Command{
		Name:      "publish",
		ShortHelp: "Publish a VCS-backed private registry module.",
		LongHelp: heredoc.New(inv.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s module publish" }} command publishes a private registry module from an existing VCS connection.

		Provide exactly one of {{ template "mdCodeOrBold" "--oauth-token-id" }} or {{ template "mdCodeOrBold" "--github-app-installation-id" }}.

		Publishing from tags is the default. Use {{ template "mdCodeOrBold" "--branch" }} to publish from a branch and optionally set its first version with {{ template "mdCodeOrBold" "--initial-version" }}.

		The command uses {{ template "mdCodeOrBold" "--repo" }} for both the VCS identifier and display identifier. Repositories that require different values, such as some Bitbucket Cloud repositories, are not supported. Use {{ template "mdCodeOrBold" "%s api" }} for those repositories.

		The module name and provider are derived from the repository name. Explicit overrides for repositories that do not follow Terraform module naming conventions are not supported.
		`, version.Name, version.Name),
		Flags: cmd.Flags{
			// These remote and repository-specific values have no reliable local predictors.
			Local: []*cmd.Flag{
				{
					Name:         "repo",
					DisplayValue: "REPOSITORY",
					Description:  "VCS repository identifier to publish.",
					Value:        flagvalue.Simple("", &opts.Repository),
					Required:     true,
				},
				{
					Name:         "organization",
					Shorthand:    "o",
					DisplayValue: "NAME",
					Description:  "Organization name (defaults to profile or Terraform cloud configuration context).",
					Value:        flagvalue.Simple((*string)(nil), &opts.Organization),
				},
				{
					Name:         "oauth-token-id",
					DisplayValue: "ID",
					Description:  "OAuth token ID for an existing VCS connection.",
					Value:        flagvalue.Simple("", &opts.OAuthTokenID),
				},
				{
					Name:         "github-app-installation-id",
					DisplayValue: "ID",
					Description:  "GitHub App installation ID for an existing VCS connection.",
					Value:        flagvalue.Simple("", &opts.GitHubAppInstallationID),
				},
				{
					Name:         "branch",
					DisplayValue: "BRANCH",
					Description:  "Branch to publish instead of publishing from tags.",
					Value:        flagvalue.Simple((*string)(nil), &opts.Branch),
				},
				{
					Name:         "initial-version",
					DisplayValue: "VERSION",
					Description:  "Initial module version for branch-based publishing.",
					Value:        flagvalue.Simple((*string)(nil), &opts.InitialVersion),
				},
			},
		},
		Examples: []cmd.Example{
			{
				Preamble: "Publish from tags with an OAuth connection:",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s module publish --repo ORG/REPO --oauth-token-id ot-...`, version.Name),
			},
			{
				Preamble: "Publish from tags with a GitHub App connection:",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s module publish --repo ORG/REPO --github-app-installation-id ghain-...`, version.Name),
			},
			{
				Preamble: "Publish from a branch with an initial version:",
				Command:  heredoc.New(inv.IO, heredoc.WithNoWrap(), heredoc.WithPreserveNewlines()).Mustf(`$ %s module publish --repo ORG/REPO --oauth-token-id ot-... --branch main --initial-version 1.0.0`, version.Name),
			},
		},
		RunF: func(_ *cmd.Command, _ []string) error {
			opts.ProfileOrganization = inv.Profile.DefaultOrganization
			opts.DryRun = inv.IsDryRun()
			opts.Quiet = inv.IsQuiet()

			apiClient, err := inv.NewAPIClient()
			if err != nil {
				return fmt.Errorf("failed to create API client: %w", err)
			}
			opts.Client = apiClient

			return runPublish(inv.ShutdownCtx, opts)
		},
	}
}

func runPublish(ctx context.Context, opts *PublishOpts) error {
	repository := strings.TrimSpace(opts.Repository)
	if repository == "" {
		return errors.New("repository is required")
	}

	organizationFlag := ""
	if opts.Organization != nil {
		organizationFlag = strings.TrimSpace(*opts.Organization)
		if organizationFlag == "" {
			return errors.New("--organization must not be blank")
		}
	}

	branch := ""
	if opts.Branch != nil {
		branch = strings.TrimSpace(*opts.Branch)
		if branch == "" {
			return errors.New("--branch must not be blank")
		}
	}

	initialVersion := ""
	if opts.InitialVersion != nil {
		initialVersion = strings.TrimSpace(*opts.InitialVersion)
		if initialVersion == "" {
			return errors.New("--initial-version must not be blank")
		}
	}

	oauthTokenID := strings.TrimSpace(opts.OAuthTokenID)
	githubAppInstallationID := strings.TrimSpace(opts.GitHubAppInstallationID)
	if (oauthTokenID == "") == (githubAppInstallationID == "") {
		return errors.New("exactly one of --oauth-token-id or --github-app-installation-id must be provided")
	}

	if initialVersion != "" && branch == "" {
		return errors.New("--initial-version requires --branch")
	}

	organization := strings.TrimSpace(cmdutil.ResolveOrganization(
		strings.TrimSpace(opts.ProfileOrganization),
		organizationFlag,
	))
	path, err := cmdutil.ResolvePath(registryModulePublishPath, organization)
	if err != nil {
		return fmt.Errorf("failed to resolve registry module publish path: %w", err)
	}

	request := publishRequestEnvelope{
		Data: publishRequestData{
			Type: "registry-modules",
			Attributes: publishRequestAttributes{
				InitialVersion: initialVersion,
				VCSRepo: publishRequestVCSRepo{
					Identifier:              repository,
					DisplayIdentifier:       repository,
					OAuthTokenID:            oauthTokenID,
					GitHubAppInstallationID: githubAppInstallationID,
					Branch:                  branch,
				},
			},
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to encode registry module publish request: %w", err)
	}

	requestURL, err := client.ResolveURL(*opts.Client.BaseURL, path)
	if err != nil {
		return fmt.Errorf("failed to resolve registry module publish URL: %w", err)
	}

	publishingMode := "tag-based"
	if branch != "" {
		publishingMode = "branch-based"
	}

	if opts.DryRun {
		fmt.Fprintf(opts.IO.Err(), "%s would publish VCS-backed module from repository %q to organization %q using %s publishing\n",
			opts.IO.ColorScheme().DryRunLabel(), repository, organization, publishingMode)
		return nil
	}

	logger := logging.FromContext(ctx)
	logger.Debug("Publishing VCS-backed registry module",
		"method", http.MethodPost,
		"path", requestURL.Path,
		"organization", organization,
		"mode", publishingMode,
	)

	resp, err := opts.Client.Do(ctx, &client.Request{
		Method: http.MethodPost,
		URL:    requestURL,
		Headers: http.Header{
			"Content-Type": []string{jsonAPIContentType},
		},
		Body: body,
	})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("failed to publish registry module: %w", err)
	}

	if opts.Quiet {
		logger.Debug("Quiet mode enabled, rendering skipped")
		return nil
	}

	if resp == nil || resp.Body == nil {
		return errors.New("failed to decode registry module publish response: response body is missing")
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read registry module publish response: %w", err)
	}

	var response publishResponseEnvelope
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return fmt.Errorf("failed to decode registry module publish response: %w", err)
	}
	if response.Data == nil {
		return errors.New("failed to decode registry module publish response: data is missing")
	}

	result := publishResult{
		ID:        response.Data.ID,
		Name:      response.Data.Attributes.Name,
		Namespace: response.Data.Attributes.Namespace,
		Provider:  response.Data.Attributes.Provider,
		Status:    response.Data.Attributes.Status,
	}

	if response.Data.Links.Self != "" {
		result.SelfLink, err = resolvePublishSelfLink(opts.Client.BaseURL, response.Data.Links.Self)
		if err != nil {
			return fmt.Errorf("failed to resolve registry module self link: %w", err)
		}
	}

	if err := opts.Output.Display(&publishDisplayer{result: result}); err != nil {
		return fmt.Errorf("failed to display published registry module: %w", err)
	}

	switch strings.ToLower(result.Status) {
	case "pending", "processing":
		fmt.Fprintf(opts.IO.ErrUnessential(), "Publish request accepted. The module is still processing (status: %s) and may not be ready yet.\n", result.Status)
	}

	return nil
}

type publishRequestEnvelope struct {
	Data publishRequestData `json:"data"`
}

type publishRequestData struct {
	Type       string                   `json:"type"`
	Attributes publishRequestAttributes `json:"attributes"`
}

type publishRequestAttributes struct {
	InitialVersion string                `json:"initial-version,omitempty"`
	VCSRepo        publishRequestVCSRepo `json:"vcs-repo"`
}

type publishRequestVCSRepo struct {
	Identifier              string `json:"identifier"`
	DisplayIdentifier       string `json:"display-identifier"`
	OAuthTokenID            string `json:"oauth-token-id,omitempty"`
	GitHubAppInstallationID string `json:"github-app-installation-id,omitempty"`
	Branch                  string `json:"branch,omitempty"`
}

type publishResponseEnvelope struct {
	Data *publishResponseData `json:"data"`
}

type publishResponseData struct {
	ID         string                    `json:"id"`
	Attributes publishResponseAttributes `json:"attributes"`
	Links      publishResponseLinks      `json:"links"`
}

type publishResponseAttributes struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`
}

type publishResponseLinks struct {
	Self string `json:"self"`
}

type publishResult struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Status    string `json:"status,omitempty"`
	SelfLink  string `json:"self_link,omitempty"`
}

type publishDisplayer struct {
	result publishResult
}

var _ format.Displayer = (*publishDisplayer)(nil)

func (d *publishDisplayer) DefaultFormat() format.Format { return format.Pretty }
func (d *publishDisplayer) Payload() any                 { return d.result }
func (d *publishDisplayer) FieldTemplates() []format.Field {
	fields := make([]format.Field, 0, 6)
	if d.result.ID != "" {
		fields = append(fields, format.NewField("ID", "{{ .ID }}"))
	}
	if d.result.Name != "" {
		fields = append(fields, format.NewField("Name", "{{ .Name }}"))
	}
	if d.result.Namespace != "" {
		fields = append(fields, format.NewField("Namespace", "{{ .Namespace }}"))
	}
	if d.result.Provider != "" {
		fields = append(fields, format.NewField("Provider", "{{ .Provider }}"))
	}
	if d.result.Status != "" {
		fields = append(fields, format.NewField("Status", "{{ .Status }}"))
	}
	if d.result.SelfLink != "" {
		fields = append(fields, format.NewField("Self Link", "{{ .SelfLink }}"))
	}
	return fields
}

func resolvePublishSelfLink(base *url.URL, self string) (string, error) {
	ref, err := url.Parse(self)
	if err != nil {
		return "", err
	}
	if ref.IsAbs() {
		return self, nil
	}
	if base == nil {
		return "", errors.New("configured API origin is missing")
	}

	// A relative API link must not replace the configured origin.
	ref.Scheme = ""
	ref.Opaque = ""
	ref.User = nil
	ref.Host = ""
	origin := url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/"}
	return origin.ResolveReference(ref).String(), nil
}
