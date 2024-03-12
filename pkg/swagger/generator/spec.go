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
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"

	"github.com/go-openapi/analysis"
	swaggererrors "github.com/go-openapi/errors"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
	"gopkg.in/yaml.v2"
)

func (g *GenOpts) loadSpec() (*loads.Document, error) {
	// Load spec document
	specDoc, err := loads.Spec(g.Spec)
	if err != nil {
		return nil, err
	}
	return specDoc, nil
}

func (g *GenOpts) validateSpec(specDoc loads.Document) error {
	log.Printf("validating spec %v", g.Spec)
	validationErrors := validate.Spec(&specDoc, strfmt.Default)
	if validationErrors != nil {
		str := fmt.Sprintf("The swagger spec at %q is invalid against swagger specification %s. see errors :\n",
			g.Spec, specDoc.Version())
		for _, desc := range validationErrors.(*swaggererrors.CompositeError).Errors {
			str += fmt.Sprintf("- %s\n", desc)
		}
		return errors.New(str)
	}
	return nil
}

func (g *GenOpts) flattenSpec() (*loads.Document, error) {
	// Flatten spec
	//
	// Some preprocessing is required before codegen
	//
	// This ensures at least that $ref's in the spec document are canonical,
	// i.e all $ref are local to this file and point to some uniquely named definition.
	//
	// Default option is to ensure minimal flattening of $ref, bundling remote $refs and relocating arbitrary JSON
	// pointers as definitions.
	// This preprocessing may introduce duplicate names (e.g. remote $ref with same name). In this case, a definition
	// suffixed with "OAIGen" is produced.
	//
	// Full flattening option farther transforms the spec by moving every complex object (e.g. with some properties)
	// as a standalone definition.
	//
	// Eventually, an "expand spec" option is available. It is essentially useful for testing purposes.
	//
	// NOTE(fredbi): spec expansion may produce some unsupported constructs and is not yet protected against the
	// following cases:
	//  - polymorphic types generation may fail with expansion (expand destructs the reuse intent of the $ref in allOf)
	//  - name duplicates may occur and result in compilation failures
	//
	// The right place to fix these shortcomings is go-openapi/analysis.
	specDoc, err := g.loadSpec()
	if err != nil {
		return nil, err
	}
	g.FlattenOpts.BasePath = specDoc.SpecFilePath()
	g.FlattenOpts.Spec = analysis.New(specDoc.Spec())

	g.printFlattenOpts()

	if err := analysis.Flatten(*g.FlattenOpts); err != nil {
		return nil, err
	}

	// yields the preprocessed spec document
	return specDoc, nil
}

func (g *GenOpts) analyzeSpec() (*loads.Document, *analysis.Spec, error) {
	// preprocess: add x-order to properties
	if g.KeepOrder {
		g.Spec = WithXOrder(g.Spec, AddXOrderOnProperty)
	}

	// load spec document and validate spec if needed
	specDoc, err := g.loadSpec()
	if err != nil {
		return nil, nil, err
	}
	if g.ValidateSpec {
		err = g.validateSpec(*specDoc)
		if err != nil {
			return nil, nil, err
		}
	}

	// preprocess: add x-order to maps in "default" & "example" fields
	// this logic should run after spec validation, since x-extensions are not allowed on "default" & "example" fields
	if g.KeepOrder {
		g.Spec = WithXOrder(g.Spec, AddXOrderOnDefaultExample)
	}

	// flatten spec
	specDoc, err = g.flattenSpec()
	if err != nil {
		return nil, nil, err
	}

	// analyze the spec
	analyzed := analysis.New(specDoc.Spec())

	return specDoc, analyzed, nil
}

func (g *GenOpts) printFlattenOpts() {
	var preprocessingOption string
	switch {
	case g.FlattenOpts.Expand:
		preprocessingOption = "expand"
	case g.FlattenOpts.Minimal:
		preprocessingOption = "minimal flattening"
	default:
		preprocessingOption = "full flattening"
	}
	log.Printf("preprocessing spec with option:  %s", preprocessingOption)
}

// findSwaggerSpec fetches a default swagger spec if none is provided
func findSwaggerSpec(nm string) (string, error) {
	specs := []string{"swagger.json", "swagger.yml", "swagger.yaml"}
	if nm != "" {
		specs = []string{nm}
	}
	var name string
	for _, nn := range specs {
		f, err := os.Stat(nn)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if f.IsDir() {
			return "", fmt.Errorf("the spec path %s is a directory", nn)
		}
		name = nn
		break
	}
	if name == "" {
		return "", errors.New("couldn't find a swagger spec")
	}
	return name, nil
}

// WithXOrder amends the spec to specify the order of some fields (such as property, default, example, ...). supports yaml documents only.
func WithXOrder(specPath string, addXOrderFunc func(yamlDoc interface{}) interface{}) string {
	yamlDoc, err := swag.YAMLData(specPath)
	if err != nil {
		panic(err)
	}

	added := addXOrderFunc(yamlDoc)

	out, err := yaml.Marshal(added)
	if err != nil {
		panic(err)
	}

	tmpFile, err := os.CreateTemp("", filepath.Base(specPath))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(tmpFile.Name(), out, 0); err != nil {
		panic(err)
	}
	return tmpFile.Name()
}

// AddXOrderOnDefaultExample amends the spec to specify the map value order in "default" & "example" fields as they appear
// in the spec (supports yaml documents only).
func AddXOrderOnDefaultExample(yamlDoc interface{}) interface{} {
	lookForSlice := func(ele interface{}, key string) (interface{}, bool) {
		if slice, ok := ele.(yaml.MapSlice); ok {
			for _, v := range slice {
				if v.Key == key && reflect.ValueOf(v.Value).Kind() == reflect.Slice {
					return v.Value, ok
				}
			}
		}
		return nil, false
	}

	var addXOrder2MapValue func(interface{})
	addXOrder2MapValue = func(element interface{}) {
		value := reflect.ValueOf(element)
		switch value.Kind() {
		case reflect.Slice:
			if ele, ok := element.(yaml.MapSlice); ok {
				var newMap yaml.MapSlice
				for i, item := range ele {
					addXOrder2MapValue(item.Value)
					newMap = append(newMap, yaml.MapItem{
						Key: item.Key,
						Value: yaml.MapSlice{
							{
								Key:   "value",
								Value: item.Value,
							},
							{
								Key:   xOrder,
								Value: i,
							},
						},
					})
				}
				// update old element
				for i, item := range newMap {
					ele[i] = item
				}
				break
			}
			var newSlice []interface{}
			for i := 0; i < value.Len(); i++ {
				itemValue := value.Index(i).Interface()
				addXOrder2MapValue(itemValue)
				newSlice = append(newSlice, itemValue)
			}
			element = newSlice
		default:
			break
		}
	}
	// Add default / example x-order
	var addXOrder func(interface{})
	addXOrder = func(element interface{}) {
		// Assuming element is a certain definition, that is, a schema, first find the default and example options in it
		if defaultValue, ok := lookForSlice(element, "default"); ok {
			addXOrder2MapValue(defaultValue)
		}
		if exampleValue, ok := lookForSlice(element, "example"); ok {
			addXOrder2MapValue(exampleValue)
		}
		// Look for the properties and add addXOrder on each property
		if props, ok := lookForMapSlice(element, "properties"); ok {
			for _, prop := range props {
				if pSlice, ok := prop.Value.(yaml.MapSlice); ok {
					addXOrder(pSlice)
				}
			}
		}
	}

	if defs, ok := lookForMapSlice(yamlDoc, "definitions"); ok {
		for _, def := range defs {
			addXOrder(def.Value)
		}
	}
	return yamlDoc
}

// AddXOrderOnProperty amends the spec to specify property order as they appear
// in the spec (supports yaml documents only).
func AddXOrderOnProperty(yamlDoc interface{}) interface{} {
	var addXOrder func(interface{})
	addXOrder = func(element interface{}) {
		if props, ok := lookForMapSlice(element, "properties"); ok {
			for i, prop := range props {
				if pSlice, ok := prop.Value.(yaml.MapSlice); ok {
					isObject := false
					xOrderIndex := -1 //Find if x-order already exists

					for i, v := range pSlice {
						if v.Key == "type" && v.Value == object {
							isObject = true
						}
						if v.Key == xOrder {
							xOrderIndex = i
							break
						}
					}

					if xOrderIndex > -1 { //Override existing x-order
						pSlice[xOrderIndex] = yaml.MapItem{Key: xOrder, Value: i}
					} else { // append new x-order
						pSlice = append(pSlice, yaml.MapItem{Key: xOrder, Value: i})
					}
					prop.Value = pSlice
					props[i] = prop

					if isObject {
						addXOrder(pSlice)
					}
				}
			}
		}
	}
	if defs, ok := lookForMapSlice(yamlDoc, "definitions"); ok {
		for _, def := range defs {
			addXOrder(def.Value)
		}
	}
	addXOrder(yamlDoc)
	return yamlDoc
}

func lookForMapSlice(ele interface{}, key string) (yaml.MapSlice, bool) {
	if slice, ok := ele.(yaml.MapSlice); ok {
		for _, v := range slice {
			if v.Key == key {
				if slice, ok := v.Value.(yaml.MapSlice); ok {
					return slice, ok
				}
			}
		}
	}
	return nil, false
}
