package client

import (
	"github.com/hashicorp/go-tfe/api/models"
)

// NewVarRequest creates a new varsets.ItemRelationshipsVarsPostRequestBody
// from parameters.
func NewVarRequest(key, value, category string, sensitive bool) models.VarsEnvelopeable {
	hcl := false
	attrib := &models.Vars_attributes{}
	attrib.SetKey(&key)
	attrib.SetValue(&value)
	attrib.SetSensitive(&sensitive)
	attrib.SetHcl(&hcl)
	attrib.SetCategory(mustParseCategory(category))

	data := &models.Vars{}
	data.SetAttributes(attrib)

	body := models.NewVarsEnvelope()
	body.SetData(data)

	return body
}

func mustParseCategory(category string) *models.Vars_attributes_category {
	cat, err := models.ParseVars_attributes_category(category)
	if err != nil {
		panic("cannot parse category \"" + category + "\"")
	}
	result := cat.(*models.Vars_attributes_category)
	return result
}

// NewOrganizationsVarsetsPostBody creates a new organizations.ItemVarsetsPostRequestBody
// from parameters.
func NewOrganizationsVarsetsPostBody(variableSetName string) *models.VarsetsEnvelope {
	attrib := &models.Varsets_attributes{}
	attrib.SetName(&variableSetName)

	data := &models.Varsets{}
	data.SetAttributes(attrib)
	body := &models.VarsetsEnvelope{}
	body.SetData(data)

	return body
}
