package client

import (
	"github.com/hashicorp/go-tfe/api/models"
	"github.com/hashicorp/go-tfe/api/organizations"
)

// NewVarRequest creates a new varsets.ItemRelationshipsVarsPostRequestBody
// from parameters.
func NewVarRequest(key, value, category string, sensitive bool) models.VarEnvelopeable {
	hcl := false
	attrib := &models.Var_attributes{}
	attrib.SetKey(&key)
	attrib.SetValue(&value)
	attrib.SetSensitive(&sensitive)
	attrib.SetHcl(&hcl)
	attrib.SetCategory(mustParseCategory(category))

	data := &models.VarEscaped{}
	data.SetAttributes(attrib)

	body := models.NewVarEnvelope()
	body.SetData(data)

	return body
}

func mustParseCategory(category string) *models.Var_attributes_category {
	cat, err := models.ParseVar_attributes_category(category)
	if err != nil {
		panic("cannot parse category \"" + category + "\"")
	}
	result := cat.(*models.Var_attributes_category)
	return result
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
