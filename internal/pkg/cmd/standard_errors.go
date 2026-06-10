// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/hashicorp/tfctl-cli/version"
)

// RequireOrganization requires that the profile has a set organization.
func RequireOrganization(inv *Invocation) error {
	if inv.Profile.Organization != "" {
		return nil
	}

	cs := inv.IO.ColorScheme()
	help := heredoc.Docf(`%v

	Please run %v to interactively set the Organization, or run:

	%v`,
		cs.String("Organization must be configured before running the command.").Color(cs.Orange()),
		cs.String(fmt.Sprintf("%s config init", version.Name)).Bold(),
		cs.String(fmt.Sprintf("$ %s config set organization <organization>", version.Name)).Bold(),
	)

	return errors.New(help)
}

// RequireOrg requires that the profile has a set organization.
func RequireOrg(inv *Invocation) error {
	if inv.Profile.Organization != "" {
		return nil
	}

	cs := inv.IO.ColorScheme()
	help := heredoc.Docf(`%v

	Please run %s to interactively set the Organization, or run:

	%v`,
		cs.String("Organization must be configured before running the command.").Color(cs.Orange()),
		cs.String(fmt.Sprintf("%s config init", version.Name)).Bold(),
		cs.String(fmt.Sprintf("$ %s config set organization <organization>", version.Name)).Bold(),
	)

	return errors.New(help)
}
