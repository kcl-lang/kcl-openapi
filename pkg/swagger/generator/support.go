// Copyright 2015 go-swagger maintainers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generator

import (
	"fmt"
	"log"
	"sort"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
)

func Generate(opts *GenOpts) error {
	generator, err := newGenerator(opts)
	if err != nil {
		return err
	}
	return generator.Generate()
}

func newGenerator(opts *GenOpts) (*generator, error) {
	if err := opts.CheckOpts(); err != nil {
		return nil, err
	}

	opts.setTemplates()

	specDoc, analyzed, err := opts.analyzeSpec()
	if err != nil {
		return nil, err
	}

	models, err := gatherModels(specDoc)
	if err != nil {
		return nil, err
	}

	return &generator{
		SpecDoc:       specDoc,
		Analyzed:      analyzed,
		Models:        models,
		Target:        opts.Target,
		ModelsPackage: opts.LanguageOpts.ManglePackagePath("", defaultModelsTarget),
		GenOpts:       opts,
	}, nil
}

type generator struct {
	Name          string
	SpecDoc       *loads.Document
	Analyzed      *analysis.Spec
	Package       string
	ModelsPackage string
	MainPackage   string
	Models        map[string]spec.Schema
	Target        string
	GenOpts       *GenOpts
}

func (a *generator) Generate() error {
	app, err := a.makeCodegen()
	if err != nil {
		return err
	}

	// NOTE: relative to previous implem with chan.
	// IPC removed concurrent execution because of the FuncMap that is being shared
	// templates are now lazy loaded so there is concurrent map access I can't guard

	log.Printf("rendering %d models", len(app.Models))
	for _, mod := range app.Models {
		if err := a.GenOpts.renderDefinition(&mod); err != nil {
			return err
		}
	}
	return nil
}

func (a *generator) makeCodegen() (GenApp, error) {
	log.Println("building a plan for generation")

	sw := a.SpecDoc.Spec()

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
	basePath := "/"
	if sw.BasePath != "" {
		basePath = sw.BasePath
	}
	return GenApp{
		GenCommon: GenCommon{
			Copyright:        a.GenOpts.Copyright,
			TargetImportPath: baseImport,
		},
		Package:      a.Package,
		BasePath:     basePath,
		ExternalDocs: sw.ExternalDocs,
		Info:         sw.Info,
		Models:       genModels,
		GenOpts:      a.GenOpts,
	}, nil
}
