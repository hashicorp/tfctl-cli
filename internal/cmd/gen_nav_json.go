// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// DocNavItem is a single item in the navigation JSON.
type DocNavItem struct {
	Title  string        `json:"title"`
	Href   string        `json:"href,omitempty"`
	Path   string        `json:"path,omitempty"`
	Routes []*DocNavItem `json:"routes,omitempty"`
}

// GenNavJSON generates the navigation JSON in the format that web-unified-docs expects,
// for the command structure.
func GenNavJSON(c *Command, w io.Writer) error {

	root := &DocNavItem{}
	genNavJSON(c, root, "cli/commands")

	// Create the top level nav item
	nav := &DocNavItem{
		Title:  "Commands (CLI)",
		Routes: root.Routes[0].Routes,
	}

	// Serialize the JSON
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	if err := e.Encode(nav); err != nil {
		return err
	}

	return nil
}

// genNavJSON is a recursive function that generates the navigation JSON for
// the command structure.
func genNavJSON(c *Command, nav *DocNavItem, path string) {
	// Generate a new nav item for this command
	var self *DocNavItem

	if c.parent != nil {
		path = filepath.Join(path, c.Name)
	}

	// Handle being a command group
	if len(c.children) > 0 {
		self = &DocNavItem{
			Title: c.Name,
			Routes: []*DocNavItem{
				{
					Title: "Overview",
					Path:  path,
				},
			},
		}
	} else {
		self = &DocNavItem{
			Title: c.Name,
			Path:  path,
		}
	}

	// Sort the children by name
	slices.SortFunc(c.children, func(i, j *Command) int {
		return strings.Compare(i.Name, j.Name)
	})

	// If we have children, create a new nav item for each child
	for _, child := range c.children {
		genNavJSON(child, self, path)
	}

	nav.Routes = append(nav.Routes, self)
}
