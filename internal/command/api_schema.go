package command

// APISchemaCommand displays API schema information.
type APISchemaCommand struct {
	// Meta provides UI and stream access for command execution.
	Meta *Meta
}

// Synopsis returns a short summary of the command.
func (c *APISchemaCommand) Synopsis() string { return "Display API schema information" }

// Help returns the command help text.
func (c *APISchemaCommand) Help() string {
	return ""
}

// Run executes the API schema command.
func (c *APISchemaCommand) Run(_ []string) int {
	c.Meta.UI.Error("not implemented")
	return 1
}
