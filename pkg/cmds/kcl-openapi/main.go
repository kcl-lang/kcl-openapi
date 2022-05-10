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

var (
	// Debug is true when the SWAGGER_DEBUG env var is not empty
	Debug = os.Getenv("SWAGGER_DEBUG") != ""
)

var opts struct {
	// General options applicable to all commands
	Quiet   func()       `long:"quiet" short:"q" description:"silence logs"`
	LogFile func(string) `long:"log-output" description:"redirect logs to file" value-name:"LOG-FILE"`
	// Version bool `long:"version" short:"v" description:"print the version of the command"`
}

func Main() {
	parser := flags.NewParser(&opts, flags.Default)
	parser.ShortDescription = "helps you keep your API well described"
	parser.LongDescription = `
Swagger tries to support you as best as possible when building APIs.

It aims to represent the contract of your API with a language agnostic description of your application in json or yaml.
`
	_, err := parser.AddCommand("validate", "validate the swagger document", "validate the provided swagger document against a swagger spec", &cmds.ValidateSpec{})
	if err != nil {
		log.Fatal(err)
	}

	genpar, err := parser.AddCommand("generate", "generate kcl code", "generate kcl code for the swagger spec file", &cmds.Generate{})
	if err != nil {
		log.Fatalln(err)
	}
	for _, cmd := range genpar.Commands() {
		switch cmd.Name {
		case "model":
			cmd.ShortDescription = "generate one or more models from the swagger spec"
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
