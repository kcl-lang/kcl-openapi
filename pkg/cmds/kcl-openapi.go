package cmds

import (
	"io/ioutil"
	"log"
	"os"

	crdGen "kcl-lang.io/kcl-openapi/pkg/kube_resource/generator"
	"kcl-lang.io/kcl-openapi/pkg/swagger/generator"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/loads/fmts"
	"github.com/jessevdk/go-flags"
)

const version = "0.5.0"

func init() {
	loads.AddLoader(fmts.YAMLMatcher, fmts.YAMLDoc)
}

var opts struct {
	// General options applicable to all commands
	Quiet   func()       `long:"quiet" short:"q" description:"silence logs"`
	LogFile func(string) `long:"log-output" description:"redirect logs to file" value-name:"LOG-FILE"`
	Version func()       `long:"version" short:"v" description:"print the version of kcl-openapi"`
}

// Generate command to group all generator commands together
type Generate struct {
	Model *Model `command:"model"`
}

// Model is the generate model file command
type Model struct {
	Options options
}

type options struct {
	Spec                 flags.Filename `long:"spec" short:"f" description:"the path to the OpenAPI spec file. It should be a local path in your file system" group:"shared"`
	Crd                  bool           `long:"crd" description:"if the spec file is a kubernetes CRD" group:"shared"`
	Target               flags.Filename `long:"target" short:"t" default:"./" description:"the base directory for generating the files" group:"shared"`
	SkipValidation       bool           `long:"skip-validation" description:"skips validation of spec prior to generation" group:"shared"`
	ModelPackage         string         `long:"model-package" short:"m" description:"the package to save the models" default:"models"`
	DisableKeepSpecOrder bool           `long:"disable-keep-spec-order" description:"disable to keep schema properties order identical to spec file"`
}

func Main() {
	parser := flags.NewParser(&opts, flags.Default)
	parser.ShortDescription = "helps you to maintain your KCL API automatically"
	parser.LongDescription = `kcl-openapi helps you to generate your KCL model code from OpenAPI spec or Kubernetes CRD.`

	genpar, err := parser.AddCommand("generate", "generate KCL code", "generate kcl code from the OpenAPI spec file", &Generate{})
	if err != nil {
		log.Fatalln(err)
	}
	for _, cmd := range genpar.Commands() {
		switch cmd.Name {
		case "model":
			cmd.ShortDescription = "generate KCL models from OpenAPI spec"
			cmd.LongDescription = cmd.ShortDescription
		}
	}
	opts.Version = func() {
		println("kcl-openapi", version)
		os.Exit(0)
	}
	opts.Quiet = func() {
		log.SetOutput(ioutil.Discard)
	}
	opts.LogFile = func(logfile string) {
		f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			log.Fatalf("cannot write to file %s: %v", logfile, err)
		}
		log.SetOutput(f)
	}

	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}
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
