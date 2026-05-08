// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestOutputter_SetFormat(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	io := iostreams.Test()
	out := format.New(io)

	// Create our displayer default to pretty printing
	d := &KVDisplayer{
		KVs: []*KV{
			{
				Key:   "Hello",
				Value: "World!",
			},
		},
		Default: format.Pretty,
	}

	// Force the format to JSON
	out.SetFormat(format.JSON)

	// Display the table
	r.NoError(out.Display(d))

	// Ensure we can unmarshal the output as JSON
	var parsed *KV
	r.NoError(json.Unmarshal(io.Output.Bytes(), &parsed))
	r.Equal(d.KVs[0], parsed)
}

type InnerL2Struct struct {
	Name string
}

type InnerL1Struct struct {
	Name  string
	Inner *InnerL2Struct
}

type OuterStruct struct {
	Name  string
	Inner *InnerL1Struct
}

func TestNilInnerStruct(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	kv := &OuterStruct{
		Name: "OuterStruct",
		// we leave inner nil on purpose
	}

	io := iostreams.Test()
	out := format.New(io)
	err := out.Show(kv, format.Pretty)

	r.NoError(err)
	fmt.Println(io.Output.String())
	r.Equal("Name:             OuterStruct\nInner Name:       \nInner Inner Name: \n", io.Output.String())
}

func TestNilInnerL2Struct(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	kv := &OuterStruct{
		Name: "OuterStruct",
		Inner: &InnerL1Struct{
			Name: "InnerL1Struct",
			// we leave inner nil on purpose
		},
	}

	io := iostreams.Test()
	out := format.New(io)
	err := out.Show(kv, format.Pretty)

	r.NoError(err)
	fmt.Println(io.Output.String())
	r.Equal("Name:             OuterStruct\nInner Name:       InnerL1Struct\nInner Inner Name: \n", io.Output.String())
}

func TestNonNilInnerStruct(t *testing.T) {
	t.Parallel()
	r := require.New(t)

	kv := &OuterStruct{
		Name: "OuterStruct",
		Inner: &InnerL1Struct{
			Name: "InnerL1Struct",
			Inner: &InnerL2Struct{
				Name: "InnerStruct",
			},
		},
	}

	io := iostreams.Test()
	out := format.New(io)
	err := out.Show(kv, format.Pretty)

	r.NoError(err)
	fmt.Println(io.Output.String())
	r.Equal("Name:             OuterStruct\nInner Name:       InnerL1Struct\nInner Inner Name: InnerStruct\n", io.Output.String())
}

func TestWithJSONAPIResource(t *testing.T) {
	t.Parallel()
	r := require.New(t)
	j := `{
  "data": {
    "id": "user-V3R563qtJNcExAkN",
    "type": "users",
    "attributes": {
      "username": "admin",
      "is-service-account": false,
      "auth-method": "tfc",
      "avatar-url": "https://www.gravatar.com/avatar/9babb00091b97b9ce9538c45807fd35f?s=100&d=mm",
      "v2-only": false,
      "is-site-admin": true,
      "is-sso-login": false,
      "email": "admin@hashicorp.com",
      "unconfirmed-email": null,
      "permissions": {
        "can-create-organizations": true,
        "can-change-email": true,
        "can-change-username": true
      },
			"double-nested": [
				{
					"key": "foo",
					"value": "bar"
				},
				{
					"key": "fizz",
					"value": "buzz"
				}
			],
			"triple-nested": {
				"level1": {
					"level2": {
						"key": "hello",
						"value": "world"
					}
				}
			},
      "tags": ["networking", "production", "us-east"]
    },
    "relationships": {
      "authentication-tokens": {
        "links": {
          "related": "/api/v2/users/user-V3R563qtJNcExAkN/authentication-tokens"
        }
      },
      "authenticated-resource": {
        "data": {
          "id": "user-V3R563qtJNcExAkN",
          "type": "users"
        },
        "links": {
          "related": "/api/v2/users/user-V3R563qtJNcExAkN"
        }
      }
    },
    "links": {
      "self": "/api/v2/users/user-V3R563qtJNcExAkN"
    }
  }
}`

	disp, err := format.NewJSONAPIDisplayer([]byte(j))
	r.NoError(err)

	io := iostreams.Test()
	out := format.New(io)
	out.SetFormat(format.Pretty)
	err = out.Display(disp)

	r.NoError(err)
	fmt.Println(io.Output.String())

	expected := `Id:                                   user-V3R563qtJNcExAkN
Auth Method:                          tfc
Avatar Url:                           https://www.gravatar.com/avatar/9babb00091b97b9ce9538c45807fd35f?s=100&d=mm
Email:                                admin@hashicorp.com
Is Service Account:                   false
Is Site Admin:                        true
Is Sso Login:                         false
Unconfirmed Email:                    <no value>
Username:                             admin
V2 Only:                              false
Double Nested.0.Key:                  foo
Double Nested.0.Value:                bar
Double Nested.1.Key:                  fizz
Double Nested.1.Value:                buzz
Permissions.Can Change Email:         true
Permissions.Can Change Username:      true
Permissions.Can Create Organizations: true
Tags.0:                               networking
Tags.1:                               production
Tags.2:                               us-east
Triple Nested.Level1.Level2.Key:      hello
Triple Nested.Level1.Level2.Value:    world
Type:                                 users
`
	r.Equal(expected, io.Output.String())
}
