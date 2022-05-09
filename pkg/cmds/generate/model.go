package generate

import (
	"errors"
	"log"

	crdGen "kusionstack.io/kcl-openapi/pkg/kube_resource/generator"
	"kusionstack.io/kcl-openapi/pkg/swagger/generator"
)

type modelOptions struct {
	ModelPackage               string   `long:"model-package" short:"m" description:"the package to save the models" default:"models"`
	Models                     []string `long:"model" short:"M" description:"specify a model to include in generation, repeat for multiple (defaults to all)"`
	ExistingModels             string   `long:"existing-models" description:"use pre-generated models e.g. github.com/foobar/model"`
	StrictAdditionalProperties bool     `long:"strict-additional-properties" description:"disallow extra properties when additionalProperties is set to false"`
	DisableKeepSpecOrder       bool     `long:"disable-keep-spec-order" description:"disable to keep schema properties order identical to spec file"`
	AllDefinitions             bool     `long:"all-definitions" description:"generate all model definitions regardless of usage in operations"`
}

func (mo modelOptions) apply(opts *generator.GenOpts) {
	opts.ModelPackage = mo.ModelPackage
	opts.Models = mo.Models
	opts.ExistingModels = mo.ExistingModels
	opts.StrictAdditionalProperties = mo.StrictAdditionalProperties
	opts.KeepOrder = !mo.DisableKeepSpecOrder
	opts.IgnoreOperations = mo.AllDefinitions
}

// WithModels adds the model options group
type WithModels struct {
	Models modelOptions `group:"Options for model generation"`
}

// Model the generate model file command
type Model struct {
	WithShared
	WithModels

	NoStruct bool `long:"skip-struct" description:"when present will not generate the model struct"`

	Name []string `long:"name" short:"n" description:"the model to generate, repeat for multiple (defaults to all). Same as --models"`
}

func (m Model) apply(opts *generator.GenOpts) {
	m.Shared.apply(opts)
	m.Models.apply(opts)

	opts.IncludeModel = !m.NoStruct
	opts.IncludeValidator = !m.NoStruct
}

func (m Model) log(rp string) {
	log.Printf(`Generation completed!

For this generation to compile you need to have some packages in your GOPATH:

	* github.com/go-openapi/validate
	* github.com/go-openapi/strfmt

You can get these now with: go get -u -f %s/...
`, rp)
}

func (m *Model) generate(opts *generator.GenOpts) error {
	if m.Shared.Crd {
		spec, err := crdGen.Generate(&crdGen.GenOpts{
			Spec: opts.Spec,
		})
		if err != nil {
			return err
		}
		opts.Spec = spec
	}
	return generator.Generate("", append(m.Name, m.Models.Models...), opts)
}

// Execute generates a model file
func (m *Model) Execute(args []string) error {
	if m.Shared.DumpData && (len(m.Name) > 1 || len(m.Models.Models) > 1) {
		return errors.New("only 1 model at a time is supported for dumping data")
	}
	if m.Models.ExistingModels != "" {
		log.Println("warning: Ignoring existing-models flag when generating models.")
	}
	return createSwagger(m)
}
