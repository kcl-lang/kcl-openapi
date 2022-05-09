package cmds

import (
	"errors"
	"fmt"
	"log"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
)

const (
	// Output messages
	missingArgMsg  = "The validate command requires the swagger document url to be specified"
	validSpecMsg   = "\nThe swagger spec at %q is valid against swagger specification %s\n"
	invalidSpecMsg = "\nThe swagger spec at %q is invalid against swagger specification %s.\nSee errors below:\n"
	warningSpecMsg = "\nThe swagger spec at %q showed up some valid but possibly unwanted constructs."
)

// ValidateSpec is a command that validates a swagger document
// against the swagger specification
type ValidateSpec struct {
	// SchemaURL string `long:"schema" description:"The schema url to use" default:"http://swagger.io/v2/schema.json"`
	SkipWarnings bool `long:"skip-warnings" description:"when present will not show up warnings upon validation"`
	StopOnError  bool `long:"stop-on-error" description:"when present will not continue validation after critical errors are found"`
}

// Execute validates the spec
func (c *ValidateSpec) Execute(args []string) error {
	if len(args) == 0 {
		return errors.New(missingArgMsg)
	}

	swaggerDoc := args[0]

	specDoc, err := loads.Spec(swaggerDoc)
	if err != nil {
		return err
	}

	// Attempts to report about all errors
	validate.SetContinueOnErrors(!c.StopOnError)

	v := validate.NewSpecValidator(specDoc.Schema(), strfmt.Default)
	result, _ := v.Validate(specDoc) // returns fully detailed result with errors and warnings

	if result.IsValid() {
		log.Printf(validSpecMsg, swaggerDoc, specDoc.Version())
	}
	if result.HasWarnings() {
		log.Printf(warningSpecMsg, swaggerDoc)
		if !c.SkipWarnings {
			log.Printf("See warnings below:\n")
			for _, desc := range result.Warnings {
				log.Printf("- WARNING: %s\n", desc.Error())
			}
		}
	}
	if result.HasErrors() {
		str := fmt.Sprintf(invalidSpecMsg, swaggerDoc, specDoc.Version())
		for _, desc := range result.Errors {
			str += fmt.Sprintf("- %s\n", desc.Error())
		}
		return errors.New(str)
	}

	return nil
}
