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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
)

//go:generate go-bindata -mode 420 -modtime 1482416923 -pkg=generator -ignore=.*\.sw? -ignore=.*\.md ./templates/...

func init() {
	// all initializations for the generator package
	debugOptions()
	initLanguage()
	initTemplateRepo()
	initTypes()
}

// DefaultSectionOpts for a given opts, this is used when no config file is passed
// and uses the embedded templates when no local override can be found
func DefaultSectionOpts(gen *GenOpts) {
	sec := gen.Sections
	if len(sec.Models) == 0 {
		sec.Models = []TemplateOpts{
			{
				Name:     "definition",
				Source:   "asset:model",
				Target:   "{{ joinFilePath .Target (toFilePath .Package) }}",
				FileName: "{{ (snakize (pascalize (.Name))) }}.k",
			},
		}
	}
	gen.Sections = sec
}

// TemplateOpts allows for codegen customization
type TemplateOpts struct {
	Name       string `mapstructure:"name"`
	Source     string `mapstructure:"source"`
	Target     string `mapstructure:"target"`
	FileName   string `mapstructure:"file_name"`
	SkipExists bool   `mapstructure:"skip_exists"`
	SkipFormat bool   `mapstructure:"skip_format"`
}

// SectionOpts allows for specifying options to customize the templates used for generation
type SectionOpts struct {
	Models []TemplateOpts `mapstructure:"models"`
}

// GenOpts the options for the generator
type GenOpts struct {
	ValidateSpec bool
	FlattenOpts  *analysis.FlattenOpts
	KeepOrder    bool

	Spec              string
	ModelPackage      string
	Target            string
	Sections          SectionOpts
	LanguageOpts      *LanguageOpts
	FlagStrategy      string
	CompatibilityMode string
	Copyright         string
}

// CheckOpts carries out some global consistency checks on options.
func (g *GenOpts) CheckOpts() error {
	if g == nil {
		return errors.New("gen opts are required")
	}

	// check the target path to output the generated files
	if !filepath.IsAbs(g.Target) {
		if _, err := filepath.Abs(g.Target); err != nil {
			return fmt.Errorf("could not locate target path %s: %v", g.Target, err)
		}
	}

	// check the oai spec file exists
	pth, err := findSwaggerSpec(g.Spec)
	if err != nil {
		return err
	}

	// ensure spec path is absolute
	g.Spec, err = filepath.Abs(pth)
	if err != nil {
		return fmt.Errorf("could not locate spec: %s", g.Spec)
	}

	return nil
}

// EnsureDefaults for these gen opts
func (g *GenOpts) EnsureDefaults() error {
	// default language func: KCL language func
	if g.LanguageOpts == nil {
		g.LanguageOpts = DefaultLanguageFunc()
	}

	// default section: set default section name for each section. only model section is used
	DefaultSectionOpts(g)

	// set defaults for flattening options
	g.FlattenOpts = &analysis.FlattenOpts{
		Minimal:      true,
		Verbose:      true,
		RemoveUnused: false,
		Expand:       false,
	}
	return nil
}

func (g *GenOpts) location(t *TemplateOpts, data interface{}) (string, string, error) {
	v := reflect.Indirect(reflect.ValueOf(data))
	fld := v.FieldByName("Name")
	var name string
	if fld.IsValid() {
		log.Println("name field", fld.String())
		name = fld.String()
	}

	fldpack := v.FieldByName("Package")
	pkg := ""
	if fldpack.IsValid() {
		log.Println("package field", fldpack.String())
		pkg = fldpack.String()
	}
	// concat schema pkg if exist
	dataPkg := v.FieldByName("Pkg")
	if dataPkg.IsValid() {
		log.Println("type pkg field", dataPkg.String())
		pkg += "." + dataPkg.String()
	}

	alias := v.FieldByName("Module")
	if alias.IsValid() && alias.String() != "" {
		log.Println("type pkg alias field", alias.String())
		name = alias.String()
	}

	var tags []string
	tagsF := v.FieldByName("Tags")
	if tagsF.IsValid() {
		tags = tagsF.Interface().([]string)
	}

	var useTags bool
	useTagsF := v.FieldByName("UseTags")
	if useTagsF.IsValid() {
		useTags = useTagsF.Interface().(bool)
	}

	funcMap := FuncMapFunc(g.LanguageOpts)

	pthTpl, err := template.New(t.Name + "-target").Funcs(funcMap).Parse(t.Target)
	if err != nil {
		return "", "", err
	}

	fNameTpl, err := template.New(t.Name + "-filename").Funcs(funcMap).Parse(t.FileName)
	if err != nil {
		return "", "", err
	}

	d := struct {
		Name, Package, Target string
		Tags                  []string
		UseTags               bool
		Context               interface{}
	}{
		Name:    name,
		Package: pkg,
		Target:  g.Target,
		Tags:    tags,
		UseTags: useTags,
		Context: data,
	}

	var pthBuf bytes.Buffer
	if e := pthTpl.Execute(&pthBuf, d); e != nil {
		return "", "", e
	}
	var fNameBuf bytes.Buffer
	if e := fNameTpl.Execute(&fNameBuf, d); e != nil {
		return "", "", e
	}
	return pthBuf.String(), fileName(fNameBuf.String()), nil
}

func (g *GenOpts) render(t *TemplateOpts, data interface{}) ([]byte, error) {
	var templ *template.Template

	if strings.HasPrefix(strings.ToLower(t.Source), "asset:") {
		tt, err := templates.Get(strings.TrimPrefix(t.Source, "asset:"))
		if err != nil {
			return nil, err
		}
		templ = tt
	}

	if templ == nil {
		// try to load from repository (and enable dependencies)
		name := swag.ToJSONName(strings.TrimSuffix(t.Source, ".gotmpl"))
		tt, err := templates.Get(name)
		if err == nil {
			templ = tt
		}
	}

	if templ == nil {
		return nil, fmt.Errorf("template %q not found", t.Source)
	}

	var tBuf bytes.Buffer
	if err := templ.Execute(&tBuf, data); err != nil {
		return nil, fmt.Errorf("template execution failed for template %s: %v", t.Name, err)
	}
	log.Printf("executed template %s", t.Source)

	return tBuf.Bytes(), nil
}

// Render template and write generated source code
// generated code is reformatted ("linted"), which gives an
// additional level of checking. If this step fails, the generated
// code is still dumped, for template debugging purposes.
func (g *GenOpts) write(t *TemplateOpts, data interface{}) error {
	dir, fname, err := g.location(t, data)
	if err != nil {
		return fmt.Errorf("failed to resolve template location for template %s: %v", t.Name, err)
	}

	if t.SkipExists && fileExists(dir, fname) {
		debugLog("skipping generation of %s because it already exists and skip_exist directive is set for %s",
			filepath.Join(dir, fname), t.Name)
		return nil
	}

	log.Printf("creating generated file %q in %q as %s", fname, dir, t.Name)
	content, err := g.render(t, data)
	if err != nil {
		return fmt.Errorf("failed rendering template data for %s: %v", t.Name, err)
	}

	if dir != "" {
		_, exists := os.Stat(dir)
		if os.IsNotExist(exists) {
			debugLog("creating directory %q for \"%s\"", dir, t.Name)
			// Directory settings consistent with file privileges.
			// Environment's umask may alter this setup
			if e := os.MkdirAll(dir, 0755); e != nil {
				return e
			}
		}
	}

	// Conditionally format the code, unless the user wants to skip
	formatted := content
	var writeerr error

	if !t.SkipFormat {
		formatted, err = g.LanguageOpts.FormatContent(filepath.Join(dir, fname), content)
		if err != nil {
			log.Printf("source formatting failed on template-generated source (%q for %s). Check that your template produces valid code", filepath.Join(dir, fname), t.Name)
			writeerr = ioutil.WriteFile(filepath.Join(dir, fname), content, 0644)
			if writeerr != nil {
				return fmt.Errorf("failed to write (unformatted) file %q in %q: %v", fname, dir, writeerr)
			}
			log.Printf("unformatted generated source %q has been dumped for template debugging purposes. DO NOT build on this source!", fname)
			return fmt.Errorf("source formatting on generated source %q failed: %v", t.Name, err)
		}
	}

	writeerr = ioutil.WriteFile(filepath.Join(dir, fname), formatted, 0644)
	if writeerr != nil {
		return fmt.Errorf("failed to write file %q in %q: %v", fname, dir, writeerr)
	}
	return err
}

func fileName(in string) string {
	ext := filepath.Ext(in)
	return swag.ToFileName(strings.TrimSuffix(in, ext)) + ext
}

func (g *GenOpts) renderDefinition(gg *GenDefinition) error {
	log.Printf("rendering %d templates for model %s", len(g.Sections.Models), gg.Name)
	for _, templ := range g.Sections.Models {
		if err := g.write(&templ, gg); err != nil {
			return err
		}
	}
	return nil
}

func (g *GenOpts) setTemplates() {
	templates.LoadDefaults()
}

func fileExists(target, name string) bool {
	_, err := os.Stat(filepath.Join(target, name))
	return !os.IsNotExist(err)
}

func gatherModels(specDoc *loads.Document) (map[string]spec.Schema, error) {
	models := make(map[string]spec.Schema)
	defs := specDoc.Spec().Definitions
	for k, v := range defs {
		models[k] = v
	}
	return models, nil
}

func trimBOM(in string) string {
	return strings.Trim(in, "\xef\xbb\xbf")
}

// gatherExtraSchemas produces a sorted list of extra schemas.
//
// ExtraSchemas are inlined types rendered in the same model file.
func gatherExtraSchemas(extraMap map[string]GenSchema) (extras GenSchemaList) {
	var extraKeys []string
	for k := range extraMap {
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		// figure out if top level validations are needed
		p := extraMap[k]
		extras = append(extras, p)
	}
	return
}

func sharedValidationsFromSchema(v spec.Schema, sg schemaGenContext) (sh sharedValidations) {
	sh = sharedValidations{
		Maximum:          v.Maximum,
		ExclusiveMaximum: v.ExclusiveMaximum,
		Minimum:          v.Minimum,
		ExclusiveMinimum: v.ExclusiveMinimum,
		MaxLength:        v.MaxLength,
		MinLength:        v.MinLength,
		Pattern:          v.Pattern,
		MaxItems:         v.MaxItems,
		MinItems:         v.MinItems,
		UniqueItems:      v.UniqueItems,
		MultipleOf:       v.MultipleOf,
		Enum:             v.Enum,
	}
	if v.Items != nil && v.Items.Schema != nil && v.Items.Schema.Pattern != "" {
		sh.ItemPattern = v.Items.Schema.Pattern
	}
	if v.AdditionalProperties != nil && v.AdditionalProperties.Schema != nil && v.AdditionalProperties.Schema.Pattern != "" {
		sh.AdditionalPropertiesPattern = v.AdditionalProperties.Schema.Pattern
	}
	for _, s := range v.AllOf {
		sh.AllOf = append(sh.AllOf, sharedValidationsFromSchema(s, sg))
	}
	for _, s := range v.AnyOf {
		sh.AnyOf = append(sh.AnyOf, sharedValidationsFromSchema(s, sg))
	}
	for _, s := range v.OneOf {
		sh.OneOf = append(sh.OneOf, sharedValidationsFromSchema(s, sg))
	}
	sh.pruneEnums(sg)
	return
}

func importAlias(pkg string) string {
	_, k := path.Split(pkg)
	return k
}
