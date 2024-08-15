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
	"encoding/json"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"strings"
	"text/template"
	"text/template/parse"
	"unicode"

	_ "embed"

	"github.com/go-openapi/inflect"
	"github.com/go-openapi/swag"
	"github.com/kr/pretty"
)

var (
	assets             map[string][]byte
	protectedTemplates map[string]bool

	// FuncMapFunc yields a map with all functions for templates
	FuncMapFunc func(*LanguageOpts) template.FuncMap

	templates *Repository
)

func initTemplateRepo() {
	FuncMapFunc = DefaultFuncMap

	// this makes the ToGoName func behave with the special
	// prefixing rule above
	swag.GoNamePrefixFunc = prefixForName

	assets = defaultAssets()
	protectedTemplates = defaultProtectedTemplates()
	templates = NewRepository(FuncMapFunc(DefaultLanguageFunc()))
}

// DefaultFuncMap yields a map with default functions for use n the templates.
// These are available in every template
func DefaultFuncMap(lang *LanguageOpts) template.FuncMap {
	return map[string]interface{}{
		"pascalize": pascalize,
		"camelize":  swag.ToJSONName,
		"varname":   lang.MangleVarName,
		"humanize":  swag.ToHumanNameLower,
		"snakize":   lang.MangleFileName,
		"toPackagePath": func(name string) string {
			path := filepath.FromSlash(lang.ManglePackagePath(name, ""))
			return path
		},
		"toPackage": func(name string) string {
			return lang.ManglePackagePath(name, "")
		},
		"toPackageName": func(name string) string {
			return lang.ManglePackageName(name, "")
		},
		"dasherize":          swag.ToCommandName,
		"pluralizeFirstWord": pluralizeFirstWord,
		"json":               asJSON,
		"prettyjson":         asPrettyJSON,
		"hasInsecure": func(arg []string) bool {
			return swag.ContainsStringsCI(arg, "http") || swag.ContainsStringsCI(arg, "ws")
		},
		"hasSecure": func(arg []string) bool {
			return swag.ContainsStringsCI(arg, "https") || swag.ContainsStringsCI(arg, "wss")
		},
		"dropPackage":    dropPackage,
		"upper":          strings.ToUpper,
		"contains":       swag.ContainsStrings,
		"padSurround":    padSurround,
		"joinFilePath":   filepath.Join,
		"comment":        padComment,
		"doc":            padDocument,
		"blockcomment":   blockComment,
		"inspect":        pretty.Sprint,
		"cleanPath":      path.Clean,
		"hasPrefix":      strings.HasPrefix,
		"stringContains": strings.Contains,
		"toFilePath": func(pkg string) string {
			path := filepath.Join(strings.Split(pkg, ".")...)
			return path
		},
		"shortType": func(def string) string {
			idx := strings.LastIndex(def, ".")
			if idx == -1 {
				return def
			}
			return def[idx+1:]
		},
		"indent": func(spaces int, v string) string {
			pad := strings.Repeat(" ", spaces)
			return pad + strings.Replace(v, "\n", "\n"+pad, -1)
		},
		"baseTypes": func(allOf GenSchemaList) GenSchemaList {
			var baseTypes GenSchemaList
			for _, one := range allOf {
				if one.IsBaseType {
					baseTypes = append(baseTypes, one)
				}
			}
			return baseTypes
		},
		"nonBaseTypes": func(allOf GenSchemaList) GenSchemaList {
			var nonBaseTypes GenSchemaList
			for _, one := range allOf {
				if !one.IsBaseType {
					nonBaseTypes = append(nonBaseTypes, one)
				}
			}
			return nonBaseTypes
		},
		"nonBaseTypeProperties": func(allOf GenSchemaList) GenSchemaList {
			var properties GenSchemaList
			for _, one := range allOf {
				if !one.IsBaseType {
					properties = append(properties, one.Properties...)
				}
			}
			return properties
		},
		"toKCLValue":    lang.ToKclValue,
		"nonEmptyValue": lang.NonEmptyValue,
	}
}

//go:embed templates/model.gotmpl
var modelTmpl string

//go:embed templates/header.gotmpl
var headerTmpl string

//go:embed templates/docstring.gotmpl
var docstringTmpl string

//go:embed templates/schema.gotmpl
var schemaTmpl string

//go:embed templates/schemabody.gotmpl
var schemaBodyTmpl string

//go:embed templates/schemavalidator.gotmpl
var schemaValidatorTmpl string

//go:embed templates/schemaexpr.gotmpl
var schemaExprTmpl string

//go:embed templates/itemsvalidator.gotmpl
var itemsValidatorTmpl string

//go:embed templates/addattrvalidator.gotmpl
var addAttrValidatorTmpl string

//go:embed templates/introduction.gotmpl
var introductionTmpl string

//go:embed templates/propertydoc.gotmpl
var propertyDocTmpl string

func defaultAssets() map[string][]byte {
	return map[string][]byte{
		// schema generation template
		"model.gotmpl":            []byte(modelTmpl),
		"header.gotmpl":           []byte(headerTmpl),
		"docstring.gotmpl":        []byte(docstringTmpl),
		"schema.gotmpl":           []byte(schemaTmpl),
		"schemabody.gotmpl":       []byte(schemaBodyTmpl),
		"schemavalidator.gotmpl":  []byte(schemaValidatorTmpl),
		"schemaexpr.gotmpl":       []byte(schemaExprTmpl),
		"itemsvalidator.gotmpl":   []byte(itemsValidatorTmpl),
		"addattrvalidator.gotmpl": []byte(addAttrValidatorTmpl),
		"introduction.gotmpl":     []byte(introductionTmpl),
		"propertydoc.gotmpl":      []byte(propertyDocTmpl),
	}
}

func defaultProtectedTemplates() map[string]bool {
	return map[string]bool{
		"dereffedSchemaType":          true,
		"docstring":                   true,
		"header":                      true,
		"mapvalidator":                true,
		"model":                       true,
		"modelvalidator":              true,
		"objectvalidator":             true,
		"primitivefieldvalidator":     true,
		"privstructfield":             true,
		"privtuplefield":              true,
		"propertyValidationDocString": true,
		"propertyvalidator":           true,
		"schema":                      true,
		"schemaBody":                  true,
		"schemaType":                  true,
		"schemabody":                  true,
		"schematype":                  true,
		"schemavalidator":             true,
		"schemaexpr":                  true,
		"serverDoc":                   true,
		"slicevalidator":              true,
		"structfield":                 true,
		"structfieldIface":            true,
		"subTypeBody":                 true,
		"swaggerJsonEmbed":            true,
		"tuplefield":                  true,
		"tuplefieldIface":             true,
		"typeSchemaType":              true,
		"validationCustomformat":      true,
		"validationPrimitive":         true,
		"validationStructfield":       true,
		"withBaseTypeBody":            true,
		"withoutBaseTypeBody":         true,
		"introduction":                true,
		"propertydoc":                 true,
	}
}

// NewRepository creates a new template repository with the provided functions defined
func NewRepository(funcs template.FuncMap) *Repository {
	repo := Repository{
		files:     make(map[string]string),
		templates: make(map[string]*template.Template),
		funcs:     funcs,
	}

	if repo.funcs == nil {
		repo.funcs = make(template.FuncMap)
	}

	return &repo
}

// Repository is the repository for the generator templates
type Repository struct {
	files         map[string]string
	templates     map[string]*template.Template
	funcs         template.FuncMap
	allowOverride bool
}

// LoadDefaults will load the embedded templates
func (t *Repository) LoadDefaults() {
	for name, asset := range assets {
		if err := t.addFile(name, string(asset), true); err != nil {
			log.Fatal(err)
		}
	}
}

func (t *Repository) addFile(name, data string, allowOverride bool) error {
	fileName := name
	name = swag.ToJSONName(strings.TrimSuffix(name, ".gotmpl"))
	templ, err := template.New(name).Funcs(t.funcs).Parse(data)

	if err != nil {
		return fmt.Errorf("failed to load template %s: %v", name, err)
	}

	// check if any protected templates are defined
	if !allowOverride && !t.allowOverride {
		for _, tmpl := range templ.Templates() {
			if protectedTemplates[tmpl.Name()] {
				return fmt.Errorf("cannot overwrite protected template %s", tmpl.Name())
			}
		}
	}

	// Add each defined template into the cache
	for _, tmpl := range templ.Templates() {
		t.files[tmpl.Name()] = fileName
		t.templates[tmpl.Name()] = tmpl.Lookup(tmpl.Name())
	}

	return nil
}

// MustGet a template by name, panics when fails
func (t *Repository) MustGet(name string) *template.Template {
	tpl, err := t.Get(name)
	if err != nil {
		panic(err)
	}
	return tpl
}

// AddFile adds a file to the repository. It will create a new template based on the filename.
// It trims the .gotmpl from the end and converts the name using swag.ToJSONName. This will strip
// directory separators and Camelcase the next letter.
// e.g validation/primitive.gotmpl will become validationPrimitive
//
// If the file contains a definition for a template that is protected the whole file will not be added
func (t *Repository) AddFile(name, data string) error {
	return t.addFile(name, data, false)
}

func findDependencies(n parse.Node) []string {

	var deps []string
	depMap := make(map[string]bool)

	if n == nil {
		return deps
	}

	switch node := n.(type) {
	case *parse.ListNode:
		if node != nil && node.Nodes != nil {
			for _, nn := range node.Nodes {
				for _, dep := range findDependencies(nn) {
					depMap[dep] = true
				}
			}
		}
	case *parse.IfNode:
		for _, dep := range findDependencies(node.BranchNode.List) {
			depMap[dep] = true
		}
		for _, dep := range findDependencies(node.BranchNode.ElseList) {
			depMap[dep] = true
		}

	case *parse.RangeNode:
		for _, dep := range findDependencies(node.BranchNode.List) {
			depMap[dep] = true
		}
		for _, dep := range findDependencies(node.BranchNode.ElseList) {
			depMap[dep] = true
		}

	case *parse.WithNode:
		for _, dep := range findDependencies(node.BranchNode.List) {
			depMap[dep] = true
		}
		for _, dep := range findDependencies(node.BranchNode.ElseList) {
			depMap[dep] = true
		}

	case *parse.TemplateNode:
		depMap[node.Name] = true
	}

	for dep := range depMap {
		deps = append(deps, dep)
	}

	return deps

}

func (t *Repository) flattenDependencies(templ *template.Template, dependencies map[string]bool) map[string]bool {
	if dependencies == nil {
		dependencies = make(map[string]bool)
	}

	deps := findDependencies(templ.Tree.Root)

	for _, d := range deps {
		if _, found := dependencies[d]; !found {

			dependencies[d] = true

			if tt := t.templates[d]; tt != nil {
				dependencies = t.flattenDependencies(tt, dependencies)
			}
		}

		dependencies[d] = true
	}

	return dependencies

}

func (t *Repository) addDependencies(templ *template.Template) (*template.Template, error) {

	name := templ.Name()

	deps := t.flattenDependencies(templ, nil)

	for dep := range deps {

		if dep == "" {
			continue
		}

		tt := templ.Lookup(dep)

		// Check if we have it
		if tt == nil {
			tt = t.templates[dep]

			// Still don't have it, return an error
			if tt == nil {
				return templ, fmt.Errorf("could not find template %s", dep)
			}
			var err error

			// Add it to the parse tree
			templ, err = templ.AddParseTree(dep, tt.Tree)

			if err != nil {
				return templ, fmt.Errorf("dependency error: %v", err)
			}

		}
	}
	return templ.Lookup(name), nil
}

// Get will return the named template from the repository, ensuring that all dependent templates are loaded.
// It will return an error if a dependent template is not defined in the repository.
func (t *Repository) Get(name string) (*template.Template, error) {
	templ, found := t.templates[name]

	if !found {
		return templ, fmt.Errorf("template doesn't exist %s", name)
	}

	return t.addDependencies(templ)
}

// DumpTemplates prints out a dump of all the defined templates, where they are defined and what their dependencies are.
func (t *Repository) DumpTemplates() {
	buf := bytes.NewBuffer(nil)
	fmt.Fprintln(buf, "\n# Templates")
	for name, templ := range t.templates {
		fmt.Fprintf(buf, "## %s\n", name)
		fmt.Fprintf(buf, "Defined in `%s`\n", t.files[name])

		if deps := findDependencies(templ.Tree.Root); len(deps) > 0 {

			fmt.Fprintf(buf, "####requires \n - %v\n\n\n", strings.Join(deps, "\n - "))
		}
		fmt.Fprintln(buf, "\n---")
	}
	log.Println(buf.String())
}

func asJSON(data interface{}) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func asPrettyJSON(data interface{}) (string, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func pluralizeFirstWord(arg string) string {
	sentence := strings.Split(arg, " ")
	if len(sentence) == 1 {
		return inflect.Pluralize(arg)
	}

	return inflect.Pluralize(sentence[0]) + " " + strings.Join(sentence[1:], " ")
}

func dropPackage(str string) string {
	parts := strings.Split(str, ".")
	return parts[len(parts)-1]
}

func padSurround(entry, padWith string, i, ln int) string {
	var res []string
	if i > 0 {
		for j := 0; j < i; j++ {
			res = append(res, padWith)
		}
	}
	res = append(res, entry)
	tot := ln - i - 1
	for j := 0; j < tot; j++ {
		res = append(res, padWith)
	}
	return strings.Join(res, ",")
}

// padDocument indent multi line document with given pad
func padDocument(str string, pad string) string {
	// get the OS name
	// set the appropriate line separator
	linebreak := "\n"
	if strings.Contains(str, "\r\n") {
		linebreak = "\r\n"
	}
	lines := strings.Split(str, linebreak)
	paddingLines := make([]string, 0, len(lines))
	for _, line := range lines {
		paddingLine := line
		if line != "" {
			paddingLine = fmt.Sprintf("%s%s", pad, line)
		}
		paddingLines = append(paddingLines, paddingLine)
	}
	// no indenting before cascading empty lines
	return strings.Join(paddingLines, linebreak)
}

func padComment(str string, pads ...string) string {
	// pads specifes padding to indent multi line comments.Defaults to one space
	pad := " "
	lines := strings.Split(str, "\n")
	if len(pads) > 0 {
		pad = strings.Join(pads, "")
	}
	return pad + strings.Join(lines[:len(lines)-1], "\n"+pad) + lines[len(lines)-1]
}

func blockComment(str string) string {
	return strings.Replace(str, "*/", "[*]/", -1)
}

func pascalize(arg string) string {
	runes := []rune(arg)
	switch len(runes) {
	case 0:
		return "Empty"
	case 1: // handle special case when we have a single rune that is not handled by swag.ToGoName
		switch runes[0] {
		case '+', '-', '#', '_': // those cases are handled differently than swag utility
			return prefixForName(arg)
		}
	}
	return swag.ToGoName(swag.ToGoName(arg)) // want to remove spaces
}

func prefixForName(arg string) string {
	first := []rune(arg)[0]
	if len(arg) == 0 || unicode.IsLetter(first) {
		return ""
	}
	switch first {
	case '+':
		return "Plus"
	case '-':
		return "Minus"
	case '#':
		return "HashTag"
		// other cases ($,@ etc..) handled by swag.ToGoName
	}
	return "Nr"
}
