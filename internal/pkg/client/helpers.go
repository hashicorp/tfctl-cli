package client

import (
	"github.com/hashicorp/go-tfe/api/organizations"
	"github.com/hashicorp/go-tfe/api/varsets"
	"github.com/hashicorp/go-tfe/api/workspaces"
)

// NewVarsetsVarsPostBody creates a new varsets.ItemRelationshipsVarsPostRequestBody
// from parameters.
func NewVarsetsVarsPostBody(key, value, category string, sensitive bool) *varsets.ItemRelationshipsVarsPostRequestBody {
	hcl := false
	attrib := &varsets.ItemRelationshipsVarsPostRequestBody_data_attributes{}
	attrib.SetKey(&key)
	attrib.SetValue(&value)
	attrib.SetSensitive(&sensitive)
	attrib.SetHcl(&hcl)

	props := make(map[string]any)
	props["category"] = category

	attrib.SetAdditionalData(props)

	data := &varsets.ItemRelationshipsVarsPostRequestBody_data{}
	data.SetAttributes(attrib)
	body := &varsets.ItemRelationshipsVarsPostRequestBody{}
	body.SetData(data)

	return body
}

// NewVarsetsVarsPatchBody creates a new varsets.ItemRelationshipsVarsItemVarsPatchRequestBody
// from parameters.
func NewVarsetsVarsPatchBody(key, value, category string, sensitive bool) *varsets.ItemRelationshipsVarsItemVarsPatchRequestBody {
	hcl := false
	attrib := &varsets.ItemRelationshipsVarsItemVarsPatchRequestBody_data_attributes{}
	attrib.SetKey(&key)
	attrib.SetValue(&value)
	attrib.SetSensitive(&sensitive)
	attrib.SetHcl(&hcl)

	props := make(map[string]any)
	props["category"] = category

	attrib.SetAdditionalData(props)

	data := &varsets.ItemRelationshipsVarsItemVarsPatchRequestBody_data{}
	data.SetAttributes(attrib)
	body := &varsets.ItemRelationshipsVarsItemVarsPatchRequestBody{}
	body.SetData(data)

	return body
}

// NewWorkspacesVarsPostBody creates a new workspaces.ItemVarsPostRequestBody from parameters.
func NewWorkspacesVarsPostBody(key, value, category string, sensitive bool) *workspaces.ItemVarsPostRequestBody {
	hcl := false
	attrib := &workspaces.ItemVarsPostRequestBody_data_attributes{}
	attrib.SetKey(&key)
	attrib.SetValue(&value)
	attrib.SetSensitive(&sensitive)
	attrib.SetHcl(&hcl)

	props := make(map[string]any)
	props["category"] = category

	attrib.SetAdditionalData(props)

	data := &workspaces.ItemVarsPostRequestBody_data{}
	data.SetAttributes(attrib)
	body := &workspaces.ItemVarsPostRequestBody{}
	body.SetData(data)

	return body
}

// NewWorkspacesVarsPatchBody creates a new workspaces.ItemVarsItemVarsPatchRequestBody
// from parameters.
func NewWorkspacesVarsPatchBody(key, value, category string, sensitive bool) *workspaces.ItemVarsItemVarsPatchRequestBody {
	hcl := false
	attrib := &workspaces.ItemVarsItemVarsPatchRequestBody_data_attributes{}
	attrib.SetKey(&key)
	attrib.SetValue(&value)
	attrib.SetSensitive(&sensitive)
	attrib.SetHcl(&hcl)

	props := make(map[string]any)
	props["category"] = category

	attrib.SetAdditionalData(props)

	data := &workspaces.ItemVarsItemVarsPatchRequestBody_data{}
	data.SetAttributes(attrib)
	body := &workspaces.ItemVarsItemVarsPatchRequestBody{}
	body.SetData(data)

	return body
}

// NewOrganizationsVarsetsPostBody creates a new organizations.ItemVarsetsPostRequestBody
// from parameters.
func NewOrganizationsVarsetsPostBody(variableSetName string) *organizations.ItemVarsetsPostRequestBody {
	attrib := &organizations.ItemVarsetsPostRequestBody_data_attributes{}
	attrib.SetName(&variableSetName)

	data := &organizations.ItemVarsetsPostRequestBody_data{}
	data.SetAttributes(attrib)
	body := &organizations.ItemVarsetsPostRequestBody{}
	body.SetData(data)

	return body
}
