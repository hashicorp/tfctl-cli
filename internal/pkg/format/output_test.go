// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package format_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	example "github.com/hashicorp/go-tfe/api/account"

	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
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

func TestWithSlice(t *testing.T) {
	t.Skip("Do we even want this type of formatting?")
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
      }
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
	thing := example.DetailsGetResponse{}
	err := json.Unmarshal([]byte(j), &thing)
	r.NoError(err)
	io := iostreams.Test()
	out := format.New(io)
	err = out.Show(thing.GetData().GetAttributes(), format.Pretty)

	r.NoError(err)
	fmt.Println(io.Output.String())

	expected := `Action UR L:                              
Created At:                               2024-08-16T18:11:19.777Z
Description:                              test description
ID:                                       00000000-0000-0000-0000-000000000000
Name:                                     Agent Smith
Request Agent Op Action Run ID:           
Request Agent Op Body:                    
Request Agent Op Group:                   Enforcements
Request Agent Op ID:                      Agent Smith
Request Custom Body:                      
Request Custom Headers:                   
Request Custom Method:                    
Request Custom UR L:                      
Request Github Enable Debug Log:          
Request Github Gh Enabled Workflow Param: 
Request Github Git Ref:                   
Request Github Inputs:                    
Request Github Install Name:              
Request Github Repository:                
Request Github Workflow ID:               
---
Action UR L:                              
Created At:                               2024-06-13T17:31:17.436Z
Description:                              Runs an action against https://hashicorp.com
ID:                                       11111111-1111-1111-1111-111111111111
Name:                                     Example
Request Agent Op Action Run ID:           
Request Agent Op Body:                    
Request Agent Op Group:                   
Request Agent Op ID:                      
Request Custom Body:                      
Request Custom Headers:                   []
Request Custom Method:                    GET
Request Custom UR L:                      https://hashicorp.com
Request Github Enable Debug Log:          
Request Github Gh Enabled Workflow Param: 
Request Github Git Ref:                   
Request Github Inputs:                    
Request Github Install Name:              
Request Github Repository:                
Request Github Workflow ID:               
---
Action UR L:                              
Created At:                               2024-08-07T21:56:00.043Z
Description:                              An action to test the variables feature.
ID:                                       22222222-2222-2222-2222-222222222222
Name:                                     Variables
Request Agent Op Action Run ID:           
Request Agent Op Body:                    
Request Agent Op Group:                   
Request Agent Op ID:                      
Request Custom Body:                      
Request Custom Headers:                   []
Request Custom Method:                    GET
Request Custom UR L:                      https://${var.company}.com
Request Github Enable Debug Log:          
Request Github Gh Enabled Workflow Param: 
Request Github Git Ref:                   
Request Github Inputs:                    
Request Github Install Name:              
Request Github Repository:                
Request Github Workflow ID:               
`
	r.Equal(expected, io.Output.String())
}
