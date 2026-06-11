// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/hashicorp/tfctl-cli/version"
)

// RequireDefaultOrganization requires that the profile has a set default organization.
func RequireDefaultOrganization(inv *Invocation) error {
	if inv.Profile.DefaultOrganization != "" {
		return nil
	}

	cs := inv.IO.ColorScheme()
	help := heredoc.Docf(`%v

	Please run %v to set the default organization`,
		cs.String("Default organization must be configured before running the command.").Color(cs.Orange()),
		cs.String(fmt.Sprintf("$ %s config set default_organization <organization>", version.Name)).Bold(),
	)

	return errors.New(help)
}
