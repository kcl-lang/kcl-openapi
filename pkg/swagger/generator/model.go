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
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
)

func makeGenDefinition(name, pkg string, schema spec.Schema, specDoc *loads.Document, opts *GenOpts) (*GenDefinition, error) {
	return makeGenDefinitionHierarchy(name, pkg, "", schema, specDoc, opts)
}

func makeGenDefinitionHierarchy(name, pkg, container string, schema spec.Schema, specDoc *loads.Document, opts *GenOpts) (*GenDefinition, error) {
	receiver := "m"
	// models are resolved in the current package
	resolver := newTypeResolver("", specDoc)
	resolver.ModelName = name
	analyzed := analysis.New(specDoc.Spec())

	di := discriminatorInfo(analyzed)

	pg := schemaGenContext{
		Path:           "",
		Name:           name,
		Receiver:       receiver,
		IndexVar:       "i",
		ValueExpr:      receiver,
		Schema:         schema,
		Required:       false,
		TypeResolver:   resolver,
		Named:          true,
		ExtraSchemas:   make(map[string]GenSchema),
		Discrimination: di,
		Container:      container,
		KeepOrder:      opts.KeepOrder,
	}
	if err := pg.makeGenSchema(); err != nil {
		return nil, fmt.Errorf("could not generate schema for %s: %v", name, err)
	}
	dsi, ok := di.Discriminators["#/definitions/"+name]
	if ok {
		pg.GenSchema.IsBaseType = true
		pg.GenSchema.IsExported = true
		pg.GenSchema.DiscriminatorField = dsi.FieldName

		if pg.GenSchema.Discriminates == nil {
			pg.GenSchema.Discriminates = make(map[string]string)
		}
		pg.GenSchema.Discriminates[name] = dsi.KclType
		pg.GenSchema.DiscriminatorValue = name

		for _, v := range dsi.Children {
			pg.GenSchema.Discriminates[v.FieldValue] = v.KclType
		}
	}

	dse, ok := di.Discriminated["#/definitions/"+name]
	if ok {
		pg.GenSchema.DiscriminatorField = dse.FieldName
		pg.GenSchema.DiscriminatorValue = dse.FieldValue
		pg.GenSchema.IsSubType = true
		knownProperties := make(map[string]struct{})

		// find the referenced definitions
		// check if it has a discriminator defined
		// when it has a discriminator get the schema and run makeGenSchema for it.
		// replace the ref with this new genschema
		swsp := specDoc.Spec()
		for i, ss := range schema.AllOf {
			ref := ss.Ref
			for ref.String() != "" {
				var rsch *spec.Schema
				var err error
				rsch, err = spec.ResolveRef(swsp, &ref)
				if err != nil {
					return nil, err
				}
				ref = rsch.Ref
				if rsch != nil && rsch.Ref.String() != "" {
					ref = rsch.Ref
					continue
				}
				ref = spec.Ref{}
				if rsch != nil && rsch.Discriminator != "" {
					gs, err := makeGenDefinitionHierarchy(strings.TrimPrefix(ss.Ref.String(), "#/definitions/"), pkg, pg.GenSchema.Name, *rsch, specDoc, opts)
					if err != nil {
						return nil, err
					}
					gs.GenSchema.IsBaseType = true
					gs.GenSchema.IsExported = true
					pg.GenSchema.AllOf[i] = gs.GenSchema
					schPtr := &(pg.GenSchema.AllOf[i])
					if schPtr.AdditionalItems != nil {
						schPtr.AdditionalItems.IsBaseType = true
					}
					if schPtr.AdditionalProperties != nil {
						schPtr.AdditionalProperties.IsBaseType = true
					}
					for j := range schPtr.Properties {
						schPtr.Properties[j].IsBaseType = true
						knownProperties[schPtr.Properties[j].Name] = struct{}{}
					}
				}
			}
		}

		// dedupe the fields
		alreadySeen := make(map[string]struct{})
		for i, ss := range pg.GenSchema.AllOf {
			var remainingProperties GenSchemaList
			for _, p := range ss.Properties {
				if _, ok := knownProperties[p.Name]; !ok || ss.IsBaseType {
					if _, seen := alreadySeen[p.Name]; !seen {
						remainingProperties = append(remainingProperties, p)
						alreadySeen[p.Name] = struct{}{}
					}
				}
			}
			pg.GenSchema.AllOf[i].Properties = remainingProperties
		}
	}

	return &GenDefinition{
		GenCommon: GenCommon{
			Copyright:        opts.Copyright,
			TargetImportPath: opts.LanguageOpts.baseImport(opts.Target),
		},
		Package:      opts.LanguageOpts.ManglePackageName(path.Base(filepath.ToSlash(pkg)), "definitions"),
		GenSchema:    pg.GenSchema,
		DependsOn:    pg.Dependencies,
		ExtraSchemas: gatherExtraSchemas(pg.ExtraSchemas),
		Imports:      collectSortedImports(pg.GenSchema),
	}, nil
}

type importStmt struct {
	ImportPath string
	AsName     string
	MustAsName bool
}

func collectSortedImports(model GenSchema) []importStmt {
	importMap := map[string]importStmt{}
	collectImports(&model, model.Pkg, importMap)
	sortedPkgPaths := make([]string, 0, len(importMap))
	sortedImports := make([]importStmt, 0, len(importMap))
	for k := range importMap {
		sortedPkgPaths = append(sortedPkgPaths, k)
	}
	sort.Strings(sortedPkgPaths)
	for _, k := range sortedPkgPaths {
		sortedImports = append(sortedImports, importMap[k])
	}
	return sortedImports
}

// getImportAsName infers the <import as> name by the context of all the existing import paths and the current pkg to be imported.
// the parent package name will be added as prefix to avoid import conflict
func getImportAsName(imp map[string]importStmt, pkg string, module string) string {
	parts := strings.Split(pkg, ".")
	asName := ""
	for i := len(parts) - 1; i >= 0; i-- {
		conflict := false
		// when conflict with other import as name, the `import as` name will be "{parentPkgName}strings.Title({PkgAlias})"
		asName = parts[i] + strings.ToTitle(asName)
		for _, v := range imp {
			if v.AsName == asName {
				conflict = true
				break
			}
		}
		if !conflict {
			return asName
		}
	}
	mangledAsName := "kclMangled" + strings.ToTitle(asName)
	for _, v := range imp {
		if v.AsName == asName {
			log.Printf("[WARN] the import paths in module %s.%s are confict, please resolve it properly", pkg, module)
		}
	}
	return mangledAsName
}

// collectImports collect import paths from the sch to the toPkg, the result will be collected to the importStmt map.
func collectImports(sch *GenSchema, toPkg string, imp map[string]importStmt) {
	if sch.Items != nil {
		collectImports(sch.Items, toPkg, imp)
		sch.KclType = "[" + sch.Items.KclType + "]"
	}
	if sch.AdditionalItems != nil {
		collectImports(sch.AdditionalItems, toPkg, imp)
	}
	if sch.Object != nil {
		collectImports(sch.Object, toPkg, imp)
	}
	if sch.Properties != nil {
		for idx := range sch.Properties {
			collectImports(&sch.Properties[idx], toPkg, imp)
		}
	}
	if sch.AdditionalProperties != nil {
		collectImports(sch.AdditionalProperties, toPkg, imp)
		sch.KclType = "{str:" + sch.AdditionalProperties.KclType + "}"
	}
	if sch.AllOf != nil {
		for idx := range sch.AllOf {
			collectImports(&sch.AllOf[idx], toPkg, imp)
		}
	}
	if sch.Pkg == toPkg || sch.Pkg == "" {
		// the model to import and to import to belong to the same package,
		// or the model to import has empty pkg(that means the model is a basic type)
		return
	}
	rootPkgName := func(pkg string) string {
		firstDot := strings.Index(pkg, ".")
		if firstDot == -1 {
			return pkg
		} else {
			return pkg[:strings.Index(pkg, ".")]
		}
	}
	// the innerPkg is the full package path within the package root, which means without the root package name as prefix
	innerPkg := sch.Pkg
	if rootPkgName(sch.Pkg) == rootPkgName(toPkg) {
		// the import pkg and the toPkg reside in the same package root
		innerPkg = sch.Pkg[strings.Index(sch.Pkg, ".")+1:]
	}
	if _, ok := imp[sch.Pkg]; !ok {
		// the package path is not imported, need to import the pkg
		asName := getImportAsName(imp, innerPkg, sch.Module)
		imp[sch.Pkg] = importStmt{
			ImportPath: innerPkg, // remove the root package name
			AsName:     asName,
			// if the package alias is conflict with other imports, use the `import as` syntax to resolve conflict.
			MustAsName: asName != sch.Pkg[strings.LastIndex(sch.Pkg, ".")+1:],
		}
	}
	// update the KclType with the import as name prefix
	sch.KclType = imp[sch.Pkg].AsName + "." + sch.KclType
}

type schemaGenContext struct {
	Required                   bool
	AdditionalProperty         bool
	Named                      bool
	RefHandled                 bool
	IsVirtual                  bool
	IsTuple                    bool
	StrictAdditionalProperties bool
	KeepOrder                  bool
	Index                      int

	Path         string
	Name         string
	ParamName    string
	Accessor     string
	Receiver     string
	IndexVar     string
	KeyVar       string
	ValueExpr    string
	Container    string
	Schema       spec.Schema
	TypeResolver *typeResolver

	GenSchema      GenSchema
	Dependencies   []string // NOTE: Dependencies is actually set nowhere
	ExtraSchemas   map[string]GenSchema
	Discriminator  *discor
	Discriminated  *discee
	Discrimination *discInfo
}

func (sg *schemaGenContext) NewArrayBranch(schema *spec.Schema) *schemaGenContext {
	debugLog("new array branch %s (model: %s)", sg.Name, sg.TypeResolver.ModelName)
	pg := sg.shallowClone()
	indexVar := pg.IndexVar
	if pg.Path == "" {
		pg.Path = indexVar
	} else {
		pg.Path = pg.Path + "." + indexVar
	}
	// check who is parent, if it's a base type then rewrite the value expression
	if sg.Discrimination != nil && sg.Discrimination.Discriminators != nil {
		_, rewriteValueExpr := sg.Discrimination.Discriminators["#/definitions/"+sg.TypeResolver.ModelName]
		if (pg.IndexVar == "i" && rewriteValueExpr) || sg.GenSchema.ElemType.IsBaseType {
			pg.ValueExpr = sg.Receiver
		}
	}
	sg.GenSchema.IsBaseType = sg.GenSchema.ElemType.HasDiscriminator
	pg.IndexVar = indexVar + "i"
	pg.ValueExpr = pg.ValueExpr + "[" + indexVar + "]"
	pg.Schema = *schema
	pg.Required = false
	if sg.IsVirtual {
		pg.TypeResolver = sg.TypeResolver.NewWithModelName(sg.TypeResolver.ModelName)
	}

	// when this is an anonymous complex object, this needs to become a ref
	return pg
}

func (sg *schemaGenContext) NewAdditionalItems(schema *spec.Schema) *schemaGenContext {
	debugLog("new additional items\n")

	pg := sg.shallowClone()
	indexVar := pg.IndexVar
	pg.Name = sg.Name + " items"
	itemsLen := 0
	if sg.Schema.Items != nil {
		itemsLen = sg.Schema.Items.Len()
	}
	var mod string
	if itemsLen > 0 {
		mod = "+" + strconv.Itoa(itemsLen)
	}
	if pg.Path == "" {
		pg.Path = "(" + indexVar + mod + ")"
	} else {
		pg.Path = pg.Path + ".(" + indexVar + mod + ")"
	}
	pg.IndexVar = indexVar
	pg.ValueExpr = sg.ValueExpr + "." + pascalize(sg.KclName()) + "Items[" + indexVar + "]"
	pg.Schema = spec.Schema{}
	if schema != nil {
		pg.Schema = *schema
	}
	pg.Required = false
	return pg
}

func (sg *schemaGenContext) NewTupleElement(schema *spec.Schema, index int) *schemaGenContext {
	debugLog("New tuple element\n")

	pg := sg.shallowClone()
	if pg.Path == "" {
		pg.Path = strconv.Itoa(index)
	} else {
		pg.Path = pg.Path + "." + strconv.Itoa(index)
	}
	pg.ValueExpr = pg.ValueExpr + ".P" + strconv.Itoa(index)

	pg.Required = true
	pg.IsTuple = true
	pg.Schema = *schema

	return pg
}

func (sg *schemaGenContext) NewSchemaBranch(name string, schema spec.Schema) *schemaGenContext {
	debugLog("new schema branch %s (parent %s)", sg.Name, sg.Container)
	pg := sg.shallowClone()
	if sg.Path == "" {
		pg.Path = name
	} else {
		pg.Path = fmt.Sprintf("%s.%s", pg.Path, name)
	}
	pg.Name = name
	pg.ValueExpr = pg.ValueExpr + "." + pascalize(kclName(&schema, name))
	pg.Schema = schema
	for _, fn := range sg.Schema.Required {
		if name == fn {
			pg.Required = true
			break
		}
	}

	if pg.Schema.Default != nil && pg.Schema.ReadOnly {
		pg.Required = true
	}
	debugLog("made new schema branch %s (parent %s)", pg.Name, pg.Container)
	return pg
}

func (sg *schemaGenContext) shallowClone() *schemaGenContext {
	debugLog("cloning context %s\n", sg.Name)
	pg := new(schemaGenContext)
	*pg = *sg
	if pg.Container == "" {
		pg.Container = sg.Name
	}
	pg.GenSchema = GenSchema{}
	pg.Dependencies = nil
	pg.Named = false
	pg.Index = 0
	pg.IsTuple = false
	pg.StrictAdditionalProperties = sg.StrictAdditionalProperties
	pg.KeepOrder = sg.KeepOrder
	return pg
}

func (sg *schemaGenContext) NewCompositionBranch(schema spec.Schema, index int) *schemaGenContext {
	debugLog("new composition branch %s (parent: %s, index: %d)", sg.Name, sg.Container, index)
	pg := sg.shallowClone()
	pg.Schema = schema
	pg.Name = "AO" + strconv.Itoa(index)
	if sg.Name != sg.TypeResolver.ModelName {
		pg.Name = sg.Name + pg.Name
	}
	pg.Index = index
	debugLog("made new composition branch %s (parent: %s)", pg.Name, pg.Container)
	return pg
}

func (sg *schemaGenContext) NewAdditionalProperty(schema spec.Schema) *schemaGenContext {
	debugLog("new additional property %s (expr: %s)", sg.Name, sg.ValueExpr)
	pg := sg.shallowClone()
	pg.Schema = schema
	if pg.KeyVar == "" {
		pg.ValueExpr = sg.ValueExpr
	}
	pg.KeyVar += "k"
	pg.ValueExpr += "[" + pg.KeyVar + "]"
	pg.Path = pg.KeyVar
	pg.GenSchema.Suffix = "Value"
	if sg.Path != "" {
		pg.Path = sg.Path + "." + pg.KeyVar
	}
	return pg
}

func hasSliceValidations(model *spec.Schema) (hasSliceValidations bool) {
	hasSliceValidations = model.MaxItems != nil || model.MinItems != nil || model.UniqueItems
	return
}

func hasValidations(model *spec.Schema) (hasValidation bool) {
	hasNumberValidation := model.Maximum != nil || model.Minimum != nil || model.MultipleOf != nil
	hasStringValidation := model.MaxLength != nil || model.MinLength != nil || model.Pattern != ""
	hasValidation = hasNumberValidation || hasStringValidation || hasSliceValidations(model)
	return
}

// handleFormatConflicts handles all conflicting model properties when a format is set
func handleFormatConflicts(model *spec.Schema) {
	switch model.Format {
	case "date", "datetime", "uuid", "bsonobjectid", "base64", "duration":
		model.MinLength = nil
		model.MaxLength = nil
		model.Pattern = ""
		// more cases should be inserted here if they arise
	}
}

func (sg *schemaGenContext) schemaValidations() sharedValidations {
	model := sg.Schema
	// resolve any conflicting properties if the model has a format
	handleFormatConflicts(&model)
	s := sharedValidationsFromSchema(model, *sg)

	s.HasValidations = hasValidations(&model)
	s.HasSliceValidations = hasSliceValidations(&model)
	return s
}

func mergeValidation(other *schemaGenContext) bool {
	// NOTE: NeesRequired and NeedsValidation are deprecated
	if other.GenSchema.AdditionalProperties != nil && other.GenSchema.AdditionalProperties.HasValidations {
		return true
	}
	if other.GenSchema.AdditionalItems != nil && other.GenSchema.AdditionalItems.HasValidations {
		return true
	}
	for _, sch := range other.GenSchema.AllOf {
		if sch.HasValidations {
			return true
		}
	}
	return other.GenSchema.HasValidations
}

func (sg *schemaGenContext) MergeResult(other *schemaGenContext, liftsRequired bool) {
	sg.GenSchema.HasValidations = sg.GenSchema.HasValidations || mergeValidation(other)

	if liftsRequired && other.GenSchema.AdditionalProperties != nil && other.GenSchema.AdditionalProperties.Required {
		sg.GenSchema.Required = true
	}
	if liftsRequired && other.GenSchema.Required {
		sg.GenSchema.Required = other.GenSchema.Required
	}
	if other.GenSchema.HasBaseType {
		sg.GenSchema.HasBaseType = other.GenSchema.HasBaseType
	}
	sg.Dependencies = append(sg.Dependencies, other.Dependencies...)

	// lift extra schemas
	for k, v := range other.ExtraSchemas {
		sg.ExtraSchemas[k] = v
	}
}

func (sg *schemaGenContext) buildProperties() error {
	debugLog("building properties %s (parent: %s)", sg.Name, sg.Container)

	for k, v := range sg.Schema.Properties {
		debugLogAsJSON("building property %s[%q] (tup: %t) (BaseType: %t)",
			sg.Name, k, sg.IsTuple, sg.GenSchema.IsBaseType, sg.Schema)
		debugLog("property %s[%q] (tup: %t) HasValidations: %t)",
			sg.Name, k, sg.IsTuple, sg.GenSchema.HasValidations)

		// check if this requires de-anonymizing, if so lift this as a new struct and extra schema
		tpe, err := sg.TypeResolver.ResolveSchema(&v, true, sg.IsTuple || swag.ContainsStrings(sg.Schema.Required, k))
		if err != nil {
			return err
		}

		vv := v
		if tpe.IsComplexObject && tpe.IsAnonymous && len(v.Properties) > 0 {
			// this is an anonymous complex construct: build a new type for it
			pg := sg.makeNewSchema(sg.Name+swag.ToGoName(k), v)
			pg.IsTuple = sg.IsTuple
			if sg.Path == "" {
				pg.Path = k
			} else {
				pg.Path = fmt.Sprintf("%s.%s", pg.Path, k)
			}
			if err := pg.makeGenSchema(); err != nil {
				return err
			}
			if v.Discriminator != "" {
				pg.GenSchema.IsBaseType = true
				pg.GenSchema.IsExported = true
				pg.GenSchema.HasBaseType = true
			}

			vv = *spec.RefProperty("#/definitions/" + pg.Name)
			sg.ExtraSchemas[pg.Name] = pg.GenSchema
			// NOTE: MergeResult lifts validation status and extra schemas
			sg.MergeResult(pg, false)
		}

		emprop := sg.NewSchemaBranch(k, vv)
		emprop.IsTuple = sg.IsTuple
		if err := emprop.makeGenSchema(); err != nil {
			return err
		}

		if emprop.Schema.Ref.String() != "" {
			// expand the schema of this property, so we take informed decisions about its type
			ref := emprop.Schema.Ref
			var sch *spec.Schema
			for ref.String() != "" {
				var rsch *spec.Schema
				var err error
				specDoc := sg.TypeResolver.Doc
				rsch, err = spec.ResolveRef(specDoc.Spec(), &ref)
				if err != nil {
					return err
				}
				ref = rsch.Ref
				if rsch != nil && rsch.Ref.String() != "" {
					ref = rsch.Ref
					continue
				}
				ref = spec.Ref{}
				sch = rsch
			}

			if emprop.Discrimination != nil {
				if _, ok := emprop.Discrimination.Discriminators[emprop.Schema.Ref.String()]; ok {
					emprop.GenSchema.IsBaseType = true
					emprop.GenSchema.HasBaseType = true
				}
				if _, ok := emprop.Discrimination.Discriminated[emprop.Schema.Ref.String()]; ok {
					emprop.GenSchema.IsSubType = true
				}
			}

			// set property name
			var nm = filepath.Base(emprop.Schema.Ref.GetURL().Fragment)
			tr := sg.TypeResolver.NewWithModelName(kclName(&emprop.Schema, swag.ToGoName(nm)))
			_, err := tr.ResolveSchema(sch, false, true)
			if err != nil {
				return err
			}
			// lift validations
			if hasValidations(sch) {
				emprop.GenSchema.HasValidations = true
			}
		}

		if emprop.GenSchema.IsBaseType {
			sg.GenSchema.HasBaseType = true
		}
		sg.MergeResult(emprop, false)
		emprop.GenSchema.Extensions = emprop.Schema.Extensions
		sg.GenSchema.Properties = append(sg.GenSchema.Properties, emprop.GenSchema)
	}
	sort.Sort(sg.GenSchema.Properties)
	return nil
}

func (sg *schemaGenContext) buildAllOf() error {
	if len(sg.Schema.AllOf) == 0 {
		return nil
	}

	var hasArray, hasNonArray int
	sort.Sort(sg.GenSchema.AllOf)
	if sg.Container == "" {
		sg.Container = sg.Name
	}
	debugLogAsJSON("building all of for %d entries", len(sg.Schema.AllOf), sg.Schema)
	for i, sch := range sg.Schema.AllOf {
		tpe, ert := sg.TypeResolver.ResolveSchema(&sch, sch.Ref.String() == "", false)
		if ert != nil {
			return ert
		}

		// check for multiple arrays in allOf branches.
		// Although a valid JSON-Schema construct, it is not suited for serialization.
		// This is the same if we attempt to serialize an array with another object.
		// We issue a generation warning on this.
		if tpe.IsArray {
			hasArray++
		} else {
			hasNonArray++
		}
		debugLogAsJSON("trying", sch)
		if (tpe.IsAnonymous && len(sch.AllOf) > 0) || (sch.Ref.String() == "" && !tpe.IsComplexObject && (tpe.IsArray || tpe.IsPrimitive)) {
			// cases where anonymous structures cause the creation of a new type:
			// - nested allOf: this one is itself a AllOf: build a new type for it
			// - anonymous simple types for edge cases: array, primitive, interface{}
			// NOTE: when branches are aliased or anonymous, the nullable property in the branch type is lost.
			name := swag.ToVarName(kclName(&sch, sg.Name+"AllOf"+strconv.Itoa(i)))
			debugLog("building anonymous nested allOf in %s: %s", sg.Name, name)
			ng := sg.makeNewSchema(name, sch)
			if err := ng.makeGenSchema(); err != nil {
				return err
			}

			newsch := spec.RefProperty("#/definitions/" + ng.Name)
			sg.Schema.AllOf[i] = *newsch

			pg := sg.NewCompositionBranch(*newsch, i)
			if err := pg.makeGenSchema(); err != nil {
				return err
			}

			// lift extra schemas & validations from new type
			pg.MergeResult(ng, true)

			// add the newly created type to the list of schemas to be rendered inline
			pg.ExtraSchemas[ng.Name] = ng.GenSchema
			sg.MergeResult(pg, true)
			sg.GenSchema.AllOf = append(sg.GenSchema.AllOf, pg.GenSchema)
			continue
		}

		comprop := sg.NewCompositionBranch(sch, i)
		if err := comprop.makeGenSchema(); err != nil {
			return err
		}
		if comprop.GenSchema.IsMap && comprop.GenSchema.HasAdditionalProperties && comprop.GenSchema.AdditionalProperties != nil {
			// the anonymous branch is a map for AdditionalProperties: rewrite value expression
			comprop.GenSchema.ValueExpression = comprop.GenSchema.ValueExpression + "." + comprop.Name
			comprop.GenSchema.AdditionalProperties.ValueExpression = comprop.GenSchema.ValueExpression + "[" + comprop.GenSchema.AdditionalProperties.KeyVar + "]"
		}
		sg.MergeResult(comprop, true)
		sg.GenSchema.AllOf = append(sg.GenSchema.AllOf, comprop.GenSchema)
	}
	if hasArray > 1 || (hasArray > 0 && hasNonArray > 0) {
		log.Printf("warning: cannot generate serializable allOf with conflicting array definitions in %s", sg.Container)
	}
	return nil
}

type mapStack struct {
	Type     *spec.Schema
	Next     *mapStack
	Previous *mapStack
	ValueRef *schemaGenContext
	Context  *schemaGenContext
	NewObj   *schemaGenContext
}

func newMapStack(context *schemaGenContext) (first, last *mapStack, err error) {
	ms := &mapStack{
		Type:    &context.Schema,
		Context: context,
	}

	l := ms
	for l.HasMore() {
		tpe, err := l.Context.TypeResolver.ResolveSchema(l.Type.AdditionalProperties.Schema, true, true)
		if err != nil {
			return nil, nil, err
		}

		if !tpe.IsMap {
			//reached the end of the rabbit hole
			if tpe.IsComplexObject && tpe.IsAnonymous {
				// found an anonymous object: create the struct from a newly created definition
				nw := l.Context.makeNewSchema(l.Context.Name+" Anon", *l.Type.AdditionalProperties.Schema)
				sch := spec.RefProperty("#/definitions/" + nw.Name)
				l.NewObj = nw

				l.Type.AdditionalProperties.Schema = sch
				l.ValueRef = l.Context.NewAdditionalProperty(*sch)
			}
			// other cases where to stop are: a $ref or a simple object
			break
		}

		// continue digging for maps
		l.Next = &mapStack{
			Previous: l,
			Type:     l.Type.AdditionalProperties.Schema,
			Context:  l.Context.NewAdditionalProperty(*l.Type.AdditionalProperties.Schema),
		}
		l = l.Next
	}

	//return top and bottom entries of this stack of AdditionalProperties
	return ms, l, nil
}

// Build rewinds the stack of additional properties, building schemas from bottom to top
func (mt *mapStack) Build() error {
	if mt.NewObj == nil && mt.ValueRef == nil && mt.Next == nil && mt.Previous == nil {
		csch := mt.Type.AdditionalProperties.Schema
		cp := mt.Context.NewAdditionalProperty(*csch)
		d := mt.Context.TypeResolver.Doc

		asch, err := analysis.Schema(analysis.SchemaOpts{
			Root:     d.Spec(),
			BasePath: d.SpecFilePath(),
			Schema:   csch,
		})
		if err != nil {
			return err
		}
		cp.Required = !asch.IsSimpleSchema && !asch.IsMap

		// when the schema is an array or an alias, this may result in inconsistent
		// nullable status between the map element and the array element (resp. the aliased type).
		//
		// Example: when an object has no property and only additionalProperties,
		// which turn out to be arrays of some other object.

		// save the initial override
		if err := cp.makeGenSchema(); err != nil {
			return err
		}

		// if we have an override at the top of stack, propagates it down nested arrays
		if cp.GenSchema.IsArray {
			// do it for nested arrays: override is also about map[string][][]... constructs
			it := &cp.GenSchema
			for it.Items != nil && it.IsArray {
				it = it.Items
			}
		}

		mt.Context.MergeResult(cp, false)
		mt.Context.GenSchema.AdditionalProperties = &cp.GenSchema
		return nil
	}
	cur := mt
	for cur != nil {
		if cur.NewObj != nil {
			// a new model has been created during the stack construction (new ref on anonymous object)
			if err := cur.NewObj.makeGenSchema(); err != nil {
				return err
			}
		}

		if cur.ValueRef != nil {
			if err := cur.ValueRef.makeGenSchema(); err != nil {
				return nil
			}
		}

		if cur.NewObj != nil {
			// newly created model from anonymous object is declared as extra schema
			cur.Context.MergeResult(cur.NewObj, false)

			// propagates extra schemas
			cur.Context.ExtraSchemas[cur.NewObj.Name] = cur.NewObj.GenSchema
		}

		if cur.ValueRef != nil {
			// this is the genSchema for this new anonymous AdditionalProperty
			if err := cur.Context.makeGenSchema(); err != nil {
				return err
			}

			// if there is a ValueRef, we must have a NewObj (from newMapStack() construction)
			cur.ValueRef.GenSchema.HasValidations = cur.NewObj.GenSchema.HasValidations
			cur.Context.MergeResult(cur.ValueRef, false)
			cur.Context.GenSchema.AdditionalProperties = &cur.ValueRef.GenSchema
		}

		if cur.Previous != nil {
			// we have a parent schema: build a schema for current AdditionalProperties
			if err := cur.Context.makeGenSchema(); err != nil {
				return err
			}
		}
		if cur.Next != nil {
			// we previously made a child schema: lifts things from that one
			// - Required is not lifted (in a cascade of maps, only the last element is actually checked for Required)
			cur.Context.MergeResult(cur.Next.Context, false)
			cur.Context.GenSchema.AdditionalProperties = &cur.Next.Context.GenSchema
		}
		if cur.ValueRef != nil {
			cur.Context.MergeResult(cur.ValueRef, false)
			cur.Context.GenSchema.AdditionalProperties = &cur.ValueRef.GenSchema
		}
		cur = cur.Previous
	}

	return nil
}

func (mt *mapStack) HasMore() bool {
	return mt.Type.AdditionalProperties != nil && (mt.Type.AdditionalProperties.Schema != nil || mt.Type.AdditionalProperties.Allows)
}

func (sg *schemaGenContext) buildAdditionalProperties() error {
	if sg.Schema.AdditionalProperties == nil {
		return nil
	}
	addp := *sg.Schema.AdditionalProperties

	wantsAdditional := addp.Schema != nil || addp.Allows
	sg.GenSchema.HasAdditionalProperties = wantsAdditional
	if !wantsAdditional {
		return nil
	}

	// flag swap
	if sg.GenSchema.IsComplexObject {
		sg.GenSchema.IsAdditionalProperties = true
		sg.GenSchema.IsComplexObject = false
		sg.GenSchema.IsMap = false
	}

	if addp.Schema == nil {
		// this is for AdditionalProperties:true|false
		if addp.Allows {
			// additionalProperties: true is rendered as: map[string]interface{}
			addp.Schema = &spec.Schema{}

			addp.Schema.Typed("object", "")
			sg.GenSchema.HasAdditionalProperties = true
			sg.GenSchema.IsComplexObject = false
			sg.GenSchema.IsMap = true

			sg.GenSchema.ValueExpression += "." + swag.ToGoName(sg.Name+" additionalProperties")
			cp := sg.NewAdditionalProperty(*addp.Schema)
			cp.Name += "AdditionalProperties"
			cp.Required = false
			if err := cp.makeGenSchema(); err != nil {
				return err
			}
			sg.MergeResult(cp, false)
			sg.GenSchema.AdditionalProperties = &cp.GenSchema
			debugLog("added interface{} schema for additionalProperties[allows == true]")
		}
		return nil
	}

	if !sg.GenSchema.IsMap && (sg.GenSchema.IsAdditionalProperties && sg.Named) {
		// we have a complex object with an AdditionalProperties schema

		tpe, ert := sg.TypeResolver.ResolveSchema(addp.Schema, addp.Schema.Ref.String() == "", false)
		if ert != nil {
			return ert
		}

		if tpe.IsComplexObject && tpe.IsAnonymous {
			// if the AdditionalProperties is an anonymous complex object, generate a new type for it
			pg := sg.makeNewSchema(sg.Name+" Anon", *addp.Schema)
			if err := pg.makeGenSchema(); err != nil {
				return err
			}
			sg.MergeResult(pg, false)
			sg.ExtraSchemas[pg.Name] = pg.GenSchema

			sg.Schema.AdditionalProperties.Schema = spec.RefProperty("#/definitions/" + pg.Name)
			sg.IsVirtual = true

			comprop := sg.NewAdditionalProperty(*sg.Schema.AdditionalProperties.Schema)
			if err := comprop.makeGenSchema(); err != nil {
				return err
			}

			comprop.GenSchema.Required = true
			comprop.GenSchema.HasValidations = true

			comprop.GenSchema.ValueExpression = sg.GenSchema.ValueExpression + "." + swag.ToGoName(sg.GenSchema.Name) + "[" + comprop.KeyVar + "]"

			sg.GenSchema.AdditionalProperties = &comprop.GenSchema
			sg.GenSchema.HasAdditionalProperties = true
			sg.GenSchema.ValueExpression += "." + swag.ToGoName(sg.GenSchema.Name)

			sg.MergeResult(comprop, false)

			return nil
		}

		// this is a regular named schema for AdditionalProperties
		sg.GenSchema.ValueExpression += "." + swag.ToGoName(sg.GenSchema.Name)
		comprop := sg.NewAdditionalProperty(*addp.Schema)
		d := sg.TypeResolver.Doc
		asch, err := analysis.Schema(analysis.SchemaOpts{
			Root:     d.Spec(),
			BasePath: d.SpecFilePath(),
			Schema:   addp.Schema,
		})
		if err != nil {
			return err
		}
		comprop.Required = !asch.IsSimpleSchema && !asch.IsMap
		if err := comprop.makeGenSchema(); err != nil {
			return err
		}

		sg.MergeResult(comprop, false)
		sg.GenSchema.AdditionalProperties = &comprop.GenSchema
		sg.GenSchema.AdditionalProperties.ValueExpression = sg.GenSchema.ValueExpression + "[" + comprop.KeyVar + "]"

		// rewrite value expression for arrays and arrays of arrays in maps (rendered as map[string][][]...)
		if sg.GenSchema.AdditionalProperties.IsArray {
			// maps of slices are where an override may take effect
			sg.GenSchema.AdditionalProperties.Items.ValueExpression = sg.GenSchema.ValueExpression + "[" + comprop.KeyVar + "]" + "[" + sg.GenSchema.AdditionalProperties.IndexVar + "]"
			ap := sg.GenSchema.AdditionalProperties.Items
			for ap != nil && ap.IsArray {
				ap.Items.ValueExpression = ap.ValueExpression + "[" + ap.IndexVar + "]"
				ap = ap.Items
			}
		}
		return nil
	}

	if sg.GenSchema.IsMap && wantsAdditional {
		// this is itself an AdditionalProperties schema with some AdditionalProperties.
		// this also runs for aliased map types (with zero properties save additionalProperties)
		//
		// find out how deep this rabbit hole goes
		// descend, unwind and rewrite
		// This needs to be depth first, so it first goes as deep as it can and then
		// builds the result in reverse order.
		_, ls, err := newMapStack(sg)
		if err != nil {
			return err
		}
		return ls.Build()
	}

	if sg.GenSchema.IsAdditionalProperties && !sg.Named {
		// for an anonymous object, first build the new object
		// and then replace the current one with a $ref to the
		// new object
		newObj := sg.makeNewSchema(sg.GenSchema.Name+" P"+strconv.Itoa(sg.Index), sg.Schema)
		if err := newObj.makeGenSchema(); err != nil {
			return err
		}

		sg.GenSchema = GenSchema{}
		sg.Schema = *spec.RefProperty("#/definitions/" + newObj.Name)
		if err := sg.makeGenSchema(); err != nil {
			return err
		}
		sg.MergeResult(newObj, false)

		sg.GenSchema.HasValidations = newObj.GenSchema.HasValidations
		sg.ExtraSchemas[newObj.Name] = newObj.GenSchema
		return nil
	}
	return nil
}

func (sg *schemaGenContext) makeNewSchema(name string, schema spec.Schema) *schemaGenContext {
	debugLog("making new schema: name: %s, container: %s", name, sg.Container)
	sp := sg.TypeResolver.Doc.Spec()
	name = swag.ToGoName(name)
	if sg.TypeResolver.ModelName != sg.Name {
		name = swag.ToGoName(sg.TypeResolver.ModelName + " " + name)
	}
	if sp.Definitions == nil {
		sp.Definitions = make(spec.Definitions)
	}
	sp.Definitions[name] = schema
	pg := schemaGenContext{
		Path:                       "",
		Name:                       name,
		Receiver:                   sg.Receiver,
		IndexVar:                   "i",
		ValueExpr:                  sg.Receiver,
		Schema:                     schema,
		Required:                   false,
		Named:                      true,
		ExtraSchemas:               make(map[string]GenSchema),
		Discrimination:             sg.Discrimination,
		Container:                  sg.Container,
		StrictAdditionalProperties: sg.StrictAdditionalProperties,
		KeepOrder:                  sg.KeepOrder,
	}
	if schema.Ref.String() == "" {
		pg.TypeResolver = sg.TypeResolver.NewWithModelName(name)
	}
	sg.ExtraSchemas[name] = pg.GenSchema
	return &pg
}

func (sg *schemaGenContext) buildArray() error {
	tpe, err := sg.TypeResolver.ResolveSchema(sg.Schema.Items.Schema, true, false)
	if err != nil {
		return err
	}

	// check if the element is a complex object, if so generate a new type for it
	if tpe.IsComplexObject && tpe.IsAnonymous {
		pg := sg.makeNewSchema(sg.Name+" items"+strconv.Itoa(sg.Index), *sg.Schema.Items.Schema)
		if err := pg.makeGenSchema(); err != nil {
			return err
		}
		sg.MergeResult(pg, false)
		sg.ExtraSchemas[pg.Name] = pg.GenSchema
		sg.Schema.Items.Schema = spec.RefProperty("#/definitions/" + pg.Name)
		sg.IsVirtual = true
		return sg.makeGenSchema()
	}

	// create the generation schema for items
	elProp := sg.NewArrayBranch(sg.Schema.Items.Schema)

	if err := elProp.makeGenSchema(); err != nil {
		return err
	}

	sg.MergeResult(elProp, false)
	sg.GenSchema.IsBaseType = elProp.GenSchema.IsBaseType
	sg.GenSchema.ItemsEnum = elProp.GenSchema.Enum
	elProp.GenSchema.Suffix = "Items"
	sg.GenSchema.KclType = "[" + elProp.GenSchema.KclType + "]"
	sg.GenSchema.IsArray = true
	schemaCopy := elProp.GenSchema
	schemaCopy.Required = false

	// validations of items
	// include format validation
	schemaCopy.HasValidations = hasValidations(sg.Schema.Items.Schema)

	// lift validations
	sg.GenSchema.HasValidations = sg.GenSchema.HasValidations || schemaCopy.HasValidations
	sg.GenSchema.HasSliceValidations = hasSliceValidations(&sg.Schema)
	sg.GenSchema.Items = &schemaCopy
	return nil
}

func (sg *schemaGenContext) buildItems() error {
	if sg.Schema.Items == nil {
		// in swagger, arrays MUST have an items schema
		return nil
	}

	// in Items spec, we have either Schema (array) or Schemas (tuple)
	presentsAsSingle := sg.Schema.Items.Schema != nil
	if presentsAsSingle && sg.Schema.AdditionalItems != nil { // unsure if this a valid of invalid schema
		return fmt.Errorf("single schema (%s) can't have additional items", sg.Name)
	}
	if presentsAsSingle {
		return sg.buildArray()
	}

	// This is a tuple, build a new model that represents this
	if sg.Named {
		sg.GenSchema.Name = sg.Name
		sg.GenSchema.EscapedName = DefaultLanguageFunc().MangleModelName(sg.GenSchema.Name)
		sg.GenSchema.KclType = sg.TypeResolver.kclTypeName(sg.Name)
		for i, s := range sg.Schema.Items.Schemas {
			elProp := sg.NewTupleElement(&s, i)

			if s.Ref.String() == "" {
				tpe, err := sg.TypeResolver.ResolveSchema(&s, s.Ref.String() == "", true)
				if err != nil {
					return err
				}
				if tpe.IsComplexObject && tpe.IsAnonymous {
					// if the tuple element is an anonymous complex object, build a new type for it
					pg := sg.makeNewSchema(sg.Name+" Items"+strconv.Itoa(i), s)
					if err := pg.makeGenSchema(); err != nil {
						return err
					}
					elProp.Schema = *spec.RefProperty("#/definitions/" + pg.Name)
					elProp.MergeResult(pg, false)
					elProp.ExtraSchemas[pg.Name] = pg.GenSchema
				}
			}

			if err := elProp.makeGenSchema(); err != nil {
				return err
			}
			sg.MergeResult(elProp, false)
			elProp.GenSchema.Name = "p" + strconv.Itoa(i)
			elProp.GenSchema.EscapedName = DefaultLanguageFunc().MangleModelName(elProp.GenSchema.Name)
			sg.GenSchema.Properties = append(sg.GenSchema.Properties, elProp.GenSchema)
			sg.GenSchema.IsTuple = true
		}
		return nil
	}

	// for an anonymous object, first build the new object
	// and then replace the current one with a $ref to the
	// new tuple object
	var sch spec.Schema
	sch.Typed("object", "")
	sch.Properties = make(map[string]spec.Schema, len(sg.Schema.Items.Schemas))
	for i, v := range sg.Schema.Items.Schemas {
		sch.Required = append(sch.Required, "P"+strconv.Itoa(i))
		sch.Properties["P"+strconv.Itoa(i)] = v
	}
	sch.AdditionalItems = sg.Schema.AdditionalItems
	tup := sg.makeNewSchema(sg.GenSchema.Name+"Tuple"+strconv.Itoa(sg.Index), sch)
	tup.IsTuple = true
	if err := tup.makeGenSchema(); err != nil {
		return err
	}
	tup.GenSchema.IsTuple = true
	tup.GenSchema.IsComplexObject = false
	tup.GenSchema.Title = tup.GenSchema.Name + " a representation of an anonymous Tuple type"
	tup.GenSchema.Description = ""
	sg.ExtraSchemas[tup.Name] = tup.GenSchema

	sg.Schema = *spec.RefProperty("#/definitions/" + tup.Name)
	if err := sg.makeGenSchema(); err != nil {
		return err
	}

	sg.MergeResult(tup, false)
	return nil
}

func (sg *schemaGenContext) buildAdditionalItems() error {
	wantsAdditionalItems :=
		sg.Schema.AdditionalItems != nil &&
			(sg.Schema.AdditionalItems.Allows || sg.Schema.AdditionalItems.Schema != nil)

	sg.GenSchema.HasAdditionalItems = wantsAdditionalItems
	if wantsAdditionalItems {
		// check if the element is a complex object, if so generate a new type for it
		tpe, err := sg.TypeResolver.ResolveSchema(sg.Schema.AdditionalItems.Schema, true, true)
		if err != nil {
			return err
		}
		if tpe.IsComplexObject && tpe.IsAnonymous {
			pg := sg.makeNewSchema(sg.Name+" Items", *sg.Schema.AdditionalItems.Schema)
			if err := pg.makeGenSchema(); err != nil {
				return err
			}
			sg.Schema.AdditionalItems.Schema = spec.RefProperty("#/definitions/" + pg.Name)
			pg.GenSchema.HasValidations = true
			sg.MergeResult(pg, false)
			sg.ExtraSchemas[pg.Name] = pg.GenSchema
		}

		it := sg.NewAdditionalItems(sg.Schema.AdditionalItems.Schema)
		// if AdditionalItems are themselves arrays, bump the index var
		if tpe.IsArray {
			it.IndexVar += "i"
		}

		if err := it.makeGenSchema(); err != nil {
			return err
		}

		sg.MergeResult(it, true)
		sg.GenSchema.AdditionalItems = &it.GenSchema
	}
	return nil
}

func (sg *schemaGenContext) buildXMLName() error {
	if sg.Schema.XML == nil {
		return nil
	}
	sg.GenSchema.XMLName = sg.Name

	if sg.Schema.XML.Name != "" {
		sg.GenSchema.XMLName = sg.Schema.XML.Name
		if sg.Schema.XML.Attribute {
			sg.GenSchema.XMLName += ",attr"
		}
	}
	return nil
}

func (sg *schemaGenContext) shortCircuitNamedRef() (bool, error) {
	// This if block ensures that a struct gets
	// rendered with the ref as embedded ref.
	//
	// NOTE: this assumes that all $ref point to a definition,
	// i.e. the spec is canonical, as guaranteed by minimal flattening.
	//
	// TODO: RefHandled is actually set nowhere
	if sg.RefHandled || !sg.Named || sg.Schema.Ref.String() == "" {
		return false, nil
	}
	debugLogAsJSON("short circuit named ref: %q", sg.Schema.Ref.String(), sg.Schema)

	// Simple aliased types (arrays, maps and primitives)
	//
	// Before deciding to make a struct with a composition branch (below),
	// check if the $ref points to a simple type or polymorphic (base) type.
	//
	// If this is the case, just realias this simple type, without creating a struct.
	asch, era := analysis.Schema(analysis.SchemaOpts{
		Root:     sg.TypeResolver.Doc.Spec(),
		BasePath: sg.TypeResolver.Doc.SpecFilePath(),
		Schema:   &sg.Schema,
	})
	if era != nil {
		return false, era
	}

	if asch.IsArray || asch.IsMap || asch.IsKnownType || asch.IsBaseType {
		tpx, ers := sg.TypeResolver.ResolveSchema(&sg.Schema, false, true)
		if ers != nil {
			return false, ers
		}
		tpe := resolvedType{}
		tpe.IsMap = asch.IsMap
		tpe.IsArray = asch.IsArray
		tpe.IsPrimitive = asch.IsKnownType
		tpe.IsComplexObject = false
		tpe.IsAnonymous = false
		tpe.IsBaseType = tpx.IsBaseType
		tpe.KclType = sg.TypeResolver.kclTypeName(path.Base(sg.Schema.Ref.String()))
		tpe.SwaggerType = tpx.SwaggerType
		sch := spec.Schema{}
		pg := sg.makeNewSchema(sg.Name, sch)
		if err := pg.makeGenSchema(); err != nil {
			return true, err
		}
		sg.MergeResult(pg, true)
		sg.GenSchema = pg.GenSchema
		sg.GenSchema.resolvedType = tpe
		sg.GenSchema.IsBaseType = tpe.IsBaseType
		return true, nil
	}
	tpe := resolvedType{}
	tpe.KclType = sg.TypeResolver.kclTypeName(sg.Name)
	tpe.SwaggerType = "object"
	tpe.IsComplexObject = true
	tpe.IsMap = false
	tpe.IsArray = false
	tpe.IsAnonymous = false
	item := sg.NewCompositionBranch(sg.Schema, 0)
	if err := item.makeGenSchema(); err != nil {
		return true, err
	}
	sg.GenSchema.resolvedType = tpe
	sg.MergeResult(item, true)
	sg.GenSchema.AllOf = append(sg.GenSchema.AllOf, item.GenSchema)
	return true, nil
}

// liftSpecialAllOf attempts to simplify the rendering of allOf constructs by lifting simple things into the current schema.
func (sg *schemaGenContext) liftSpecialAllOf() error {
	// if there is only a $ref or a primitive and an x-isnullable schema then this is a nullable pointer
	// so this should not compose several objects, just 1
	// if there is a ref with a discriminator then we look for x-class on the current definition to know
	// the value of the discriminator to instantiate the class
	if len(sg.Schema.AllOf) < 2 {
		return nil
	}
	var seenSchema int
	var schemaToLift spec.Schema

	for _, sch := range sg.Schema.AllOf {
		tpe, err := sg.TypeResolver.ResolveSchema(&sch, true, true)
		if err != nil {
			return err
		}
		if len(sch.Type) > 0 || len(sch.Properties) > 0 || sch.Ref.GetURL() != nil || len(sch.AllOf) > 0 {
			seenSchema++
			if seenSchema > 1 {
				// won't do anything if several candidates for a lift
				break
			}
			if (!tpe.IsAnonymous && tpe.IsComplexObject) || tpe.IsPrimitive {
				// lifting complex objects here results in inlined structs in the model
				schemaToLift = sch
			}
		}
	}

	if seenSchema == 1 {
		// when there only a single schema to lift in allOf, replace the schema by its allOf definition
		debugLog("lifted schema in allOf for %s", sg.Name)
		sg.Schema = schemaToLift
	}
	return nil
}

func (sg *schemaGenContext) KclName() string {
	return kclName(&sg.Schema, sg.Name)
}

func kclName(sch *spec.Schema, orig string) string {
	name, _ := sch.Extensions.GetString(xKclName)
	if name != "" {
		return name
	}
	return orig
}

func (sg *schemaGenContext) makeGenSchema() error {
	debugLogAsJSON("making gen schema (anon: %t, req: %t, tuple: %t) %s\n",
		!sg.Named, sg.Required, sg.IsTuple, sg.Name, sg.Schema)
	sg.GenSchema.IsExported = true
	sg.GenSchema.Path = sg.Path
	sg.GenSchema.IndexVar = sg.IndexVar
	sg.GenSchema.ValueExpression = sg.ValueExpr
	sg.GenSchema.KeyVar = sg.KeyVar
	sg.GenSchema.OriginalName = sg.Name
	sg.GenSchema.Name = sg.KclName()
	sg.GenSchema.EscapedName = DefaultLanguageFunc().MangleModelName(sg.GenSchema.Name)
	sg.GenSchema.Title = sg.Schema.Title
	sg.GenSchema.Description = trimBOM(sg.Schema.Description)
	sg.GenSchema.ReceiverName = sg.Receiver
	sg.GenSchema.sharedValidations = sg.schemaValidations()
	sg.GenSchema.ReadOnly = sg.Schema.ReadOnly
	sg.GenSchema.StrictAdditionalProperties = sg.StrictAdditionalProperties
	sg.GenSchema.Required = sg.Required
	sg.GenSchema.ExternalDocs = sg.Schema.ExternalDocs

	if sg.KeepOrder {
		sg.GenSchema.Default = RecoverMapValueOrder(sg.Schema.Default)
		sg.GenSchema.Example = RecoverMapValueOrder(sg.Schema.Example)
	} else {
		sg.GenSchema.Default = sg.Schema.Default
		sg.GenSchema.Example = sg.Schema.Example
	}

	var err error
	returns, err := sg.shortCircuitNamedRef()
	if err != nil {
		return err
	}
	if returns {
		return nil
	}
	debugLogAsJSON("after short circuit named ref", sg.Schema)

	if e := sg.liftSpecialAllOf(); e != nil {
		return e
	}
	debugLogAsJSON("after lifting special all of", sg.Schema)

	if sg.Container == "" {
		sg.Container = sg.GenSchema.Name
	}
	if e := sg.buildAllOf(); e != nil {
		return e
	}

	var tpe resolvedType
	tpe, err = sg.TypeResolver.ResolveSchema(&sg.Schema, !sg.Named, sg.IsTuple || sg.Required || sg.GenSchema.Required)
	if err != nil {
		return err
	}
	sg.GenSchema.resolvedType = tpe
	sg.GenSchema.IsBaseType = tpe.IsBaseType
	sg.GenSchema.HasDiscriminator = tpe.HasDiscriminator

	if e := sg.buildAdditionalProperties(); e != nil {
		return e
	}

	// rewrite value expression from top-down
	cur := &sg.GenSchema
	for cur.AdditionalProperties != nil {
		cur.AdditionalProperties.ValueExpression = cur.ValueExpression + "[" + cur.AdditionalProperties.KeyVar + "]"
		cur = cur.AdditionalProperties
	}

	prev := sg.GenSchema
	debugLogAsJSON("typed resolve, isAnonymous(%t), n: %t, t: %t, sgr: %t, sr: %t, isRequired(%t), BaseType(%t)",
		!sg.Named, sg.Named, sg.IsTuple, sg.Required, sg.GenSchema.Required,
		sg.Named || sg.IsTuple || sg.Required || sg.GenSchema.Required, sg.GenSchema.IsBaseType, sg.Schema)
	tpe, err = sg.TypeResolver.ResolveSchema(&sg.Schema, !sg.Named, sg.Named || sg.IsTuple || sg.Required || sg.GenSchema.Required)
	if err != nil {
		return err
	}
	sg.GenSchema.resolvedType = tpe
	sg.GenSchema.IsComplexObject = prev.IsComplexObject
	sg.GenSchema.IsMap = prev.IsMap
	sg.GenSchema.IsAdditionalProperties = prev.IsAdditionalProperties
	sg.GenSchema.IsBaseType = sg.GenSchema.HasDiscriminator

	if err := sg.buildProperties(); err != nil {
		return err
	}

	if err := sg.buildXMLName(); err != nil {
		return err
	}

	if err := sg.buildAdditionalItems(); err != nil {
		return err
	}

	if err := sg.buildItems(); err != nil {
		return err
	}

	sg.GenSchema.Extensions = sg.Schema.Extensions
	debugLog("finished gen schema for %q", sg.Name)
	return nil
}

func RecoverMapValueOrder(oldValue interface{}) interface{} {
	value := reflect.ValueOf(oldValue)
	switch value.Kind() {
	case reflect.Slice:
		var newSlice []interface{}
		for i := 0; i < value.Len(); i++ {
			itemValue := value.Index(i).Interface()
			RecoverMapValueOrder(itemValue)
			newSlice = append(newSlice, itemValue)
		}
		return newSlice
	case reflect.Map:
		keys := value.MapKeys()
		var newValue yaml.MapSlice = make([]yaml.MapItem, len(keys))

		for _, key := range keys {
			k := key.Interface()
			v := value.MapIndex(key).Interface()

			mapV := reflect.ValueOf(v)
			switch mapV.Kind() {
			case reflect.Map:
				var order int64
				var innerValue interface{}
				mapIter := mapV.MapRange()
				for mapIter.Next() {
					kk := mapIter.Key().String()
					if kk == xOrder {
						order = int64(mapIter.Value().Interface().(float64))
					}
					if kk == "value" {
						innerValue = mapIter.Value().Interface()
					}
				}
				newValue[order] = yaml.MapItem{
					Key:   k,
					Value: RecoverMapValueOrder(innerValue),
				}
			default:
				log.Fatalf("unexpected ordered map value: %s", v)
			}
		}
		return newValue
	default:
		return oldValue
	}
}
