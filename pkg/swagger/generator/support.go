package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
)

func Generate(name string, modelNames []string, opts *GenOpts) error {
	generator, err := newGenerator(name, modelNames, opts)
	if err != nil {
		return err
	}
	return generator.Generate()
}

func newGenerator(name string, modelNames []string, opts *GenOpts) (*generator, error) {
	if err := opts.CheckOpts(); err != nil {
		return nil, err
	}

	if err := opts.setTemplates(); err != nil {
		return nil, err
	}

	specDoc, analyzed, err := opts.analyzeSpec()
	if err != nil {
		return nil, err
	}

	models, err := gatherModels(specDoc, modelNames)
	if err != nil {
		return nil, err
	}

	opts.Name = appNameOrDefault(specDoc, name, defaultServerName)
	if opts.MainPackage == "" {
		// default target for the generated main
		opts.MainPackage = swag.ToCommandName(mainNameOrDefault(specDoc, name, defaultServerName) + "-server")
	}

	return &generator{
		Name:          opts.Name,
		Receiver:      "o",
		SpecDoc:       specDoc,
		Analyzed:      analyzed,
		Models:        models,
		Target:        opts.Target,
		DumpData:      opts.DumpData,
		ModelsPackage: opts.LanguageOpts.ManglePackagePath(opts.ModelPackage, defaultModelsTarget),
		Principal:     opts.Principal,
		GenOpts:       opts,
	}, nil
}

type generator struct {
	Name          string
	Receiver      string
	SpecDoc       *loads.Document
	Analyzed      *analysis.Spec
	Package       string
	ModelsPackage string
	MainPackage   string
	Principal     string
	Models        map[string]spec.Schema
	Target        string
	DumpData      bool
	GenOpts       *GenOpts
}

func (a *generator) Generate() error {
	app, err := a.makeCodegen()
	if err != nil {
		return err
	}

	if a.DumpData {
		return dumpData(app)
	}

	// NOTE: relative to previous implem with chan.
	// IPC removed concurrent execution because of the FuncMap that is being shared
	// templates are now lazy loaded so there is concurrent map access I can't guard
	if a.GenOpts.IncludeModel {
		log.Printf("rendering %d models", len(app.Models))
		for _, mod := range app.Models {
			mod.IncludeModel = true
			mod.IncludeValidator = true
			if err := a.GenOpts.renderDefinition(&mod); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *generator) GenerateSupport(ap *GenApp) error {
	app := ap
	if ap == nil {
		// allows for calling GenerateSupport standalone
		ca, err := a.makeCodegen()
		if err != nil {
			return err
		}
		app = &ca
	}
	return a.GenOpts.renderApplication(app)
}

func (a *generator) makeSecuritySchemes() GenSecuritySchemes {
	if a.Principal == "" {
		a.Principal = "object"
	}
	requiredSecuritySchemes := make(map[string]spec.SecurityScheme, len(a.Analyzed.RequiredSecuritySchemes()))
	for _, scheme := range a.Analyzed.RequiredSecuritySchemes() {
		if req, ok := a.SpecDoc.Spec().SecurityDefinitions[scheme]; ok && req != nil {
			requiredSecuritySchemes[scheme] = *req
		}
	}
	return gatherSecuritySchemes(requiredSecuritySchemes, a.Name, a.Principal, a.Receiver)
}

func (a *generator) makeCodegen() (GenApp, error) {
	log.Println("building a plan for generation")

	sw := a.SpecDoc.Spec()
	receiver := a.Receiver

	security := a.makeSecuritySchemes()

	log.Println("generation target", a.Target)

	baseImport := a.GenOpts.LanguageOpts.baseImport(a.Target)

	log.Println("planning definitions")

	genModels := make(GenDefinitions, 0, len(a.Models))
	for mn, m := range a.Models {
		model, err := makeGenDefinition(
			mn,
			a.ModelsPackage,
			m,
			a.SpecDoc,
			a.GenOpts,
		)
		if err != nil {
			return GenApp{}, fmt.Errorf("error in model %s while planning definitions: %v", mn, err)
		}
		if model != nil {
			if !model.External {
				genModels = append(genModels, *model)
			}
		}
	}
	sort.Sort(genModels)

	host := "localhost"
	if sw.Host != "" {
		host = sw.Host
	}

	basePath := "/"
	if sw.BasePath != "" {
		basePath = sw.BasePath
	}

	jsonb, _ := json.MarshalIndent(a.SpecDoc.OrigSpec(), "", "  ")
	flatjsonb, _ := json.MarshalIndent(a.SpecDoc.Spec(), "", "  ")

	return GenApp{
		GenCommon: GenCommon{
			Copyright:        a.GenOpts.Copyright,
			TargetImportPath: baseImport,
		},
		Package:             a.Package,
		ReceiverName:        receiver,
		Name:                a.Name,
		Host:                host,
		BasePath:            basePath,
		ExternalDocs:        sw.ExternalDocs,
		Info:                sw.Info,
		SecurityDefinitions: security,
		Models:              genModels,
		Principal:           a.Principal,
		SwaggerJSON:         generateReadableSpec(jsonb),
		FlatSwaggerJSON:     generateReadableSpec(flatjsonb),
		ExcludeSpec:         a.GenOpts.ExcludeSpec,
		GenOpts:             a.GenOpts,
	}, nil
}

// generateReadableSpec makes swagger json spec as a string instead of bytes
// the only character that needs to be escaped is '`' symbol, since it cannot be escaped in the GO string
// that is quoted as `string data`. The function doesn't care about the beginning or the ending of the
// string it escapes since all data that needs to be escaped is always in the middle of the swagger spec.
func generateReadableSpec(spec []byte) string {
	buf := &bytes.Buffer{}
	for _, b := range string(spec) {
		if b == '`' {
			buf.WriteString("`+\"`\"+`")
		} else {
			buf.WriteRune(b)
		}
	}
	return buf.String()
}
