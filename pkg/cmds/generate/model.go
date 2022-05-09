package generate

import (
	"log"

	"github.com/jessevdk/go-flags"

	crdGen "kusionstack.io/kcl-openapi/pkg/kube_resource/generator"
	"kusionstack.io/kcl-openapi/pkg/swagger/generator"
)

type options struct {
	Spec                 flags.Filename `long:"spec" short:"f" description:"the path to the OpenAPI spec file. It should be a local path in your file system" group:"shared"`
	Crd                  bool           `long:"crd" description:"if the spec file is a kubernetes CRD" group:"shared"`
	Target               flags.Filename `long:"target" short:"t" default:"./" description:"the base directory for generating the files" group:"shared"`
	SkipValidation       bool           `long:"skip-validation" description:"skips validation of spec prior to generation" group:"shared"`
	ModelPackage         string         `long:"model-package" short:"m" description:"the package to save the models" default:"models"`
	DisableKeepSpecOrder bool           `long:"disable-keep-spec-order" description:"disable to keep schema properties order identical to spec file"`
}

// Model is the generate model file command
type Model struct {
	Options options
}

// Execute generates a model file
func (m *Model) Execute(args []string) error {
	opts := new(generator.GenOpts)
	// cli opts to generator.GenOpts
	opts.Spec = string(m.Options.Spec)
	opts.Target = string(m.Options.Target)
	opts.ValidateSpec = !m.Options.SkipValidation
	opts.ModelPackage = m.Options.ModelPackage
	opts.KeepOrder = !m.Options.DisableKeepSpecOrder

	// set default configurations
	if err := opts.EnsureDefaults(); err != nil {
		return err
	}

	// when the spec is a crd, get openapi spec file from it
	if m.Options.Crd {
		spec, err := crdGen.GetSpec(&crdGen.GenOpts{
			Spec: opts.Spec,
		})
		if err != nil {
			return err
		}
		opts.Spec = spec
		// do not run validate spec on spec file generated from crd
		opts.ValidateSpec = false
	}

	// generate models
	if err := generator.Generate(opts); err != nil {
		return err
	}

	// generate complete
	log.Printf("Generation completed!")
	return nil
}
