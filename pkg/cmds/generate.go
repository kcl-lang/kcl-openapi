package cmds

import "kusionstack.io/kcl-openapi/pkg/cmds/generate"

// Generate command to group all generator commands together
type Generate struct {
	Model *generate.Model `command:"model"`
}
