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
	"path"
	"path/filepath"
	"strings"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
	"github.com/kr/pretty"
)

const (
	array   = "array"
	number  = "number"
	integer = "integer"
	boolean = "boolean"
	str     = "string"
	object  = "object"
	any     = "any"
)

const (
	intOrStr        = "intorstring"
	k8sIntOrStrFlag = "x-kubernetes-int-or-string"
)

// Extensions supported by go-swagger
const (
	xSchema    = "x-schema"   // schema name used by discriminator
	xKclName   = "x-kcl-name" // name of the generated kcl variable
	xKclType   = "x-kcl-type" // reuse existing type (do not generate)
	xOmitEmpty = "x-omitempty"
	xOrder     = "x-order" // sort order for properties, and "default"/"example" fields in schema
)

// swaggerTypeName contains a mapping from go type to swagger type or format
var swaggerTypeName map[string]string

func initTypes() {
	swaggerTypeName = make(map[string]string)
	for k, v := range typeMapping {
		swaggerTypeName[v] = k
	}
}

func newTypeResolver(pkg string, doc *loads.Document) *typeResolver {
	resolver := typeResolver{ModelsPackage: pkg, Doc: doc}
	resolver.KnownDefs = make(map[string]struct{}, len(doc.Spec().Definitions))
	for k, sch := range doc.Spec().Definitions {
		tpe, _, _, _ := knownDefKclType(k, sch, nil)
		resolver.KnownDefs[tpe] = struct{}{}
	}
	return &resolver
}

// knownDefKclType returns kcl type, package and package alias for definition
func knownDefKclType(def string, schema spec.Schema, clear func(string) string) (string, string, string, string) {
	debugLog("known def type: %q", def)

	ext := schema.Extensions
	if nm, ok := ext.GetString(xKclName); ok {
		if clear == nil {
			debugLog("known def type %s no clear: %q", xKclName, nm)
			return nm, "", "", ""
		}
		debugLog("known def type %s clear: %q -> %q", xKclName, nm, clear(nm))
		return clear(nm), "", "", ""
	}
	v, ok := ext[xKclType]
	if !ok {
		if clear == nil {
			debugLog("known def type no clear: %q", def)
			return def, "", "", ""
		}
		debugLog("known def type clear: %q -> %q", def, clear(def))
		return clear(def), "", "", ""
	}
	xt := v.(map[string]interface{})
	t := xt["type"].(string)
	var clearedTpe string
	if clear == nil {
		clearedTpe = t
	} else {
		clearedTpe = clear(t)
	}
	impIface, ok := xt["import"]
	if !ok {
		return clearedTpe, "", "", ""
	}
	imp := impIface.(map[string]interface{})
	pkg := imp["package"].(string)
	alias := ""
	newPkg := pkg
	// hack start
	goodIdx := strings.LastIndex(pkg, ".")
	if goodIdx != -1 {
		newPkg = pkg[:goodIdx]
	}
	goodIdx = strings.LastIndex(newPkg, ".")
	if goodIdx != -1 {
		alias = newPkg[goodIdx+1:]
	} else {
		alias = newPkg
	}
	// hack end
	var module string
	al, ok := imp["alias"]
	if ok {
		module = al.(string)
	} else {
		module = path.Base(pkg)
	}
	debugLog("known def type %s no clear: %q: pkg=%s, alias=%s, module=%s", xKclType, t, newPkg, alias, module)
	return clearedTpe, newPkg, alias, module
}

type typeResolver struct {
	Doc           *loads.Document
	ModelsPackage string
	ModelName     string
	KnownDefs     map[string]struct{}
	// unexported fields
	keepDefinitionsPkg string
	knownDefsKept      map[string]struct{}
}

// NewWithModelName clones a type resolver and specifies a new model name
func (t *typeResolver) NewWithModelName(name string) *typeResolver {
	tt := newTypeResolver(t.ModelsPackage, t.Doc)
	tt.ModelName = name

	// propagates kept definitions
	tt.keepDefinitionsPkg = t.keepDefinitionsPkg
	tt.knownDefsKept = t.knownDefsKept
	return tt
}

func (t *typeResolver) resolveSchemaRef(schema *spec.Schema, isRequired bool) (returns bool, result resolvedType, err error) {
	if schema.Ref.String() == "" {
		return
	}
	debugLog("resolving ref (anon: %t, req: %t) %s", false, isRequired, schema.Ref.String())
	returns = true
	var ref *spec.Schema
	var er error

	ref, er = spec.ResolveRef(t.Doc.Spec(), &schema.Ref)
	if er != nil {
		debugLog("error resolving ref %s: %v", schema.Ref.String(), er)
		err = er
		return
	}
	res, er := t.ResolveSchema(ref, false, isRequired)
	if er != nil {
		err = er
		return
	}
	result = res

	tn := filepath.Base(schema.Ref.GetURL().Fragment)
	tpe, pkg, alias, module := knownDefKclType(tn, *ref, t.kclTypeName)
	debugLog("type name %s, package %s, alias %s, module %s", tpe, pkg, alias, module)
	if tpe != "" {
		result.KclType = tpe
		result.Pkg = pkg
		result.PkgAlias = alias
		result.Module = module
	}
	result.HasDiscriminator = res.HasDiscriminator
	result.IsBaseType = result.HasDiscriminator
	return
}

func (t *typeResolver) resolveFormat(schema *spec.Schema, isAnonymous, isRequired bool) (returns bool, result resolvedType, err error) {
	if schema.Format != "" {
		// defaults to string
		result.SwaggerType = str
		if len(schema.Type) > 0 {
			result.SwaggerType = schema.Type[0]
		}

		debugLog("resolving format (anon: %t, req: %t)", isAnonymous, isRequired)
		schFmt := strings.Replace(schema.Format, "-", "", -1)
		if fmm, ok := formatMapping[result.SwaggerType]; ok {
			if tpe, ok := fmm[schFmt]; ok {
				returns = true
				result.KclType = tpe
			}
		}
		if tpe, ok := typeMapping[schFmt]; !returns && ok {
			returns = true
			result.KclType = tpe
		}

		result.SwaggerFormat = schema.Format
		// propagate extensions in resolvedType
		result.Extensions = schema.Extensions
	}
	return
}

func (t *typeResolver) resolveExtensions(schema *spec.Schema, isAnonymous, isRequired bool) (returns bool, result resolvedType, err error) {
	if schema.VendorExtensible.Extensions != nil {
		if value, ok := schema.VendorExtensible.Extensions.GetBool(k8sIntOrStrFlag); value && ok {
			// the schema has {"x-kubernetes-int-or-string": "true"} flag
			debugLog("resolving x-kubernetes-int-or-string type flag (anon: %t, req: %t)", isAnonymous, isRequired)
			returns = true
			result.SwaggerType = str
			if len(schema.Type) > 0 {
				result.SwaggerType = schema.Type[0]
			}
			result.KclType = typeMapping[intOrStr]
			// propagate extensions in resolvedType
			result.Extensions = schema.Extensions
		}
	}
	return
}

func (t *typeResolver) firstType(schema *spec.Schema) string {
	if len(schema.Type) == 0 || schema.Type[0] == "" {
		return object
	}
	// int or str
	if len(schema.Type) == 2 && ((schema.Type[0] == str && schema.Type[1] == integer) || (schema.Type[0] == integer && schema.Type[1] == str)) {
		return intOrStr
	}
	if len(schema.Type) > 1 {
		// JSON-Schema multiple types, e.g. {"type": [ "object", "array" ]} are not supported.
		// TODO: should keep the first _supported_ type, e.g. skip null
		log.Printf("warning: JSON-Schema type definition as array with several types is not supported in %#v. Taking the first type: %s", schema.Type, schema.Type[0])
	}
	return schema.Type[0]
}

func (t *typeResolver) resolveArray(schema *spec.Schema, isAnonymous, isRequired bool) (result resolvedType, err error) {
	debugLog("resolving array (anon: %t, req: %t)", isAnonymous, isRequired)

	result.IsArray = true
	if schema.AdditionalItems != nil {
		result.HasAdditionalItems = schema.AdditionalItems.Allows || schema.AdditionalItems.Schema != nil
	}

	if schema.Items == nil {
		result.KclType = "[" + any + "]"
		result.SwaggerType = array
		result.SwaggerFormat = ""
		return
	}

	if len(schema.Items.Schemas) > 0 {
		result.IsArray = false
		result.IsTuple = true
		result.SwaggerType = array
		result.SwaggerFormat = ""
		return
	}

	rt, er := t.ResolveSchema(schema.Items.Schema, true, false)
	if er != nil {
		err = er
		return
	}
	result.KclType = "[" + rt.KclType + "]"
	result.ElemType = &rt
	result.SwaggerType = array
	result.SwaggerFormat = ""
	result.Extensions = schema.Extensions
	return
}

func (t *typeResolver) kclTypeName(modelName string) string {
	escapedName := DefaultLanguageFunc().MangleModelName(modelName)
	if len(t.knownDefsKept) > 0 {
		// if a definitions package has been defined, already resolved definitions are
		// always resolved against their original package (e.g. "models"), and not the
		// current package.
		// This allows complex anonymous extra schemas to reuse known definitions generated in another package.
		if _, ok := t.knownDefsKept[modelName]; ok {
			return strings.Join([]string{t.keepDefinitionsPkg, escapedName}, ".")
		}
	}

	if t.ModelsPackage == "" {
		return escapedName
	}
	if _, ok := t.KnownDefs[modelName]; ok {
		return strings.Join([]string{t.ModelsPackage, escapedName}, ".")
	}
	return escapedName
}

func (t *typeResolver) resolveObject(schema *spec.Schema, isAnonymous bool) (result resolvedType, err error) {
	debugLog("resolving object %s (anon: %t, req: %t)", t.ModelName, isAnonymous, false)
	result.IsAnonymous = isAnonymous
	result.IsBaseType = schema.Discriminator != ""

	if !isAnonymous {
		result.SwaggerType = object
		tpe, pkg, alias, module := knownDefKclType(t.ModelName, *schema, t.kclTypeName)
		result.KclType = tpe
		result.Pkg = pkg
		result.PkgAlias = alias
		result.Module = module
	}
	if len(schema.AllOf) > 0 {
		result.KclType = t.kclTypeName(t.ModelName)
		result.IsComplexObject = true
		result.SwaggerType = object
		return
	}

	// if this schema has properties, build a map of property name to
	// resolved type, this should also flag the object as anonymous,
	// when a ref is found, the anonymous flag will be reset
	if len(schema.Properties) > 0 {
		result.IsComplexObject = true
		// no return here, still need to check for additional properties
	}

	// account for additional properties
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		sch := schema.AdditionalProperties.Schema
		et, er := t.ResolveSchema(sch, sch.Ref.String() == "", false)
		if er != nil {
			err = er
			return
		}
		result.IsMap = !result.IsComplexObject
		result.SwaggerType = object
		result.KclType = "{str:" + et.KclType + "}"
		result.ElemType = &et
		return
	}
	if len(schema.Properties) > 0 {
		return
	}

	if isAnonymous {
		// an anonymous object without property and without AdditionalProperties schema is rendered as object
		result.KclType = any
		result.IsMap = true
		result.SwaggerType = object
	}
	return
}

func (t *typeResolver) ResolveSchema(schema *spec.Schema, isAnonymous, isRequired bool) (result resolvedType, err error) {
	debugLog("resolving schema (anon: %t, req: %t) %s", isAnonymous, isRequired, t.ModelName)
	if schema == nil {
		result.KclType = any
		return
	}

	tpe := t.firstType(schema)
	var returns bool
	returns, result, err = t.resolveSchemaRef(schema, isRequired)
	if returns {
		if !isAnonymous {
			result.IsMap = false
			result.IsComplexObject = true
			debugLog("not anonymous ref")
		}
		debugLog("returning after ref")
		return
	}
	defer func() {
		result.setIsEmptyOmitted(schema, tpe)
	}()

	returns, result, err = t.resolveFormat(schema, isAnonymous, isRequired)
	if returns || err != nil {
		debugLog("returning after resolve format: %s", pretty.Sprint(result))
		return
	}

	returns, result, err = t.resolveExtensions(schema, isAnonymous, isRequired)
	if returns || err != nil {
		debugLog("returning after resolve vendor extensions: %s", pretty.Sprint(result))
		return
	}

	switch tpe {
	case array:
		result, err = t.resolveArray(schema, isAnonymous, false)
		return
	case number, integer, boolean, str:
		result.Extensions = schema.Extensions
		result.KclType = typeMapping[tpe]
		result.SwaggerType = tpe
		result.IsPrimitive = true
		return
	case object:
		result, err = t.resolveObject(schema, isAnonymous)
		if err != nil {
			return resolvedType{}, err
		}
		result.HasDiscriminator = schema.Discriminator != ""
		return
	default:
		err = fmt.Errorf("unresolvable: %v (format %q)", schema.Type, schema.Format)
		return
	}
}

// resolvedType is a swagger type that has been resolved and analyzed for usage
// in a template
type resolvedType struct {
	IsAnonymous    bool
	IsArray        bool
	IsMap          bool
	IsPrimitive    bool
	IsEmptyOmitted bool
	IsJSONString   bool
	IsBase64       bool

	// A tuple gets rendered as an anonymous struct with P{index} as property name
	IsTuple            bool
	HasAdditionalItems bool

	// A complex object gets rendered as a struct
	IsComplexObject bool

	// A polymorphic type
	IsBaseType       bool
	HasDiscriminator bool

	// kcl type
	KclType string
	// a kcl package
	Pkg string
	// a kcl package alias
	PkgAlias string
	// a kcl module
	Module        string
	SwaggerType   string
	SwaggerFormat string
	Extensions    spec.Extensions

	// The type of the element in a slice or map
	ElemType *resolvedType
}

func (rt *resolvedType) setIsEmptyOmitted(schema *spec.Schema, tpe string) {
	if v, found := schema.Extensions[xOmitEmpty]; found {
		omitted, cast := v.(bool)
		rt.IsEmptyOmitted = omitted && cast
		return
	}
	rt.IsEmptyOmitted = tpe != array
}
