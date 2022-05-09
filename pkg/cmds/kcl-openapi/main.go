package kcl_openapi

import (
	"io/ioutil"
	"log"
	"os"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/loads/fmts"
	"github.com/jessevdk/go-flags"

	"kusionstack.io/kcl-openapi/pkg/cmds"
)

func init() {
	loads.AddLoader(fmts.YAMLMatcher, fmts.YAMLDoc)
}

var opts struct {
	// General options applicable to all commands
	Quiet   func()       `long:"quiet" short:"q" description:"silence logs"`
	LogFile func(string) `long:"log-output" description:"redirect logs to file" value-name:"LOG-FILE"`
}

func Main() {
	parser := flags.NewParser(&opts, flags.Default)
	parser.ShortDescription = "helps you to maintain your KCL API automatically"
	parser.LongDescription = `kcl-openapi helps you to generate your KCL model code from OpenAPI spec or Kubernetes CRD.`

	genpar, err := parser.AddCommand("generate", "generate KCL code", "generate kcl code from the OpenAPI spec file", &cmds.Generate{})
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
