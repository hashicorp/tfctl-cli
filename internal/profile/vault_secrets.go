// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"errors"
	"slices"

	"github.com/posener/complete"
	"golang.org/x/exp/maps"
)

// VaultSecrets is a named set of configuration for the HCP CLI. It captures
// vault secrets related configuration values such as app name.
type VaultSecretsConf struct {
	// AppName stores the app name against which the requests will be made.
	AppName string `hcl:"app"`
}

// Predict predicts the HCL key names and basic settable values
func (vsc *VaultSecretsConf) Predict(args complete.Args) []string {
	properties := map[string][]string{
		"vault-secrets/app": {""},
	}
	// If the property has been specified, return possible values.
	if len(args.All) >= 1 {
		prediction, ok := properties[args.All[0]]
		if ok {
			return prediction
		}
	}

	// Predicting the property
	if len(args.All) == 1 {
		keys := maps.Keys(properties)
		slices.Sort(keys)
		return keys
	}
	return nil
}

// Validate validates that the set values are valid. It validates parameters
// that do not require any communication with HCP.
func (vsc *VaultSecretsConf) Validate() error {
	if vsc == nil {
		return nil
	}
	if vsc.AppName == "" {
		return errors.New("app must be set")
	}
	return nil
}

func (vsc *VaultSecretsConf) isEmpty() bool {
	if vsc == nil {
		return true
	}

	if vsc.AppName != "" {
		return false
	}

	return true
}
