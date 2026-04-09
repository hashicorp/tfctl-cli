// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"time"

	"github.com/dustin/go-humanize"

	"github.com/hashicorp/tfcloud/internal/format"
)

type Complex struct {
	Name        string
	Description string
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (c *Complex) CreatedAtHumanized() string {
	return humanize.Time(c.CreatedAt)
}

func (c *Complex) UpdatedAtHumanized() string {
	return humanize.Time(c.UpdatedAt)
}

type ComplexDisplayer struct {
	Data    []*Complex
	Default format.Format
}

func (d *ComplexDisplayer) DefaultFormat() format.Format { return d.Default }

func (d *ComplexDisplayer) Payload() any {
	if len(d.Data) == 1 {
		return d.Data[0]
	}

	return d.Data
}

func (d *ComplexDisplayer) FieldTemplates() []format.Field {
	return []format.Field{
		{
			Name:        "Name",
			ValueFormat: "{{ .Name }}",
		},
		{
			Name:        "Description",
			ValueFormat: "{{ .Description }}",
		},
		{
			Name:        "Version",
			ValueFormat: "v{{ .Version }}",
		},
		{
			Name:        "Created At",
			ValueFormat: "{{ .CreatedAtHumanized }}",
		},
		{
			Name:        "Updated At",
			ValueFormat: "{{ .UpdatedAtHumanized }}",
		},
	}
}

type KV struct {
	Key, Value string
}

type KVDisplayer struct {
	KVs     []*KV
	Default format.Format
}

func (d *KVDisplayer) DefaultFormat() format.Format { return d.Default }

func (d *KVDisplayer) Payload() any {
	if len(d.KVs) == 1 {
		return d.KVs[0]
	}

	return d.KVs
}
func (d *KVDisplayer) FieldTemplates() []format.Field {
	return []format.Field{
		{
			Name:        "Key",
			ValueFormat: "{{ .Key }}",
		},
		{
			Name:        "Value",
			ValueFormat: "{{ .Value }}",
		},
	}
}
