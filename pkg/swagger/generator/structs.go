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
	"strings"

	"github.com/go-openapi/spec"
)

// GenCommon contains common properties needed across
// definitions, app and operations
// TargetImportPath may be used by templates to import other (possibly
// generated) packages in the generation path (e.g. relative to GOPATH).
// TargetImportPath is NOT used by standard templates.
type GenCommon struct {
	Copyright        string
	TargetImportPath string
}

// GenDefinition contains all the properties to generate a
// definition from a swagger spec
type GenDefinition struct {
	GenCommon
	GenSchema
	Package      string
	Imports      []importStmt
	ExtraSchemas GenSchemaList
	DependsOn    []string
	External     bool
}

// GenDefinitions represents a list of operations to generate
// this implements a sort by operation id
type GenDefinitions []GenDefinition

func (g GenDefinitions) Len() int           { return len(g) }
func (g GenDefinitions) Less(i, j int) bool { return g[i].Name < g[j].Name }
func (g GenDefinitions) Swap(i, j int)      { g[i], g[j] = g[j], g[i] }

// GenSchemaList is a list of schemas for generation.
//
// It can be sorted by name to get a stable struct layout for
// version control and such
type GenSchemaList []GenSchema

// GenSchema contains all the information needed to generate the code
// for a schema
type GenSchema struct {
	resolvedType
	sharedValidations
	Example                    interface{}
	OriginalName               string
	Name                       string
	EscapedName                string
	Suffix                     string
	Path                       string
	ValueExpression            string
	IndexVar                   string
	KeyVar                     string
	Title                      string
	Description                string
	ReceiverName               string
	Items                      *GenSchema
	AllowsAdditionalItems      bool
	HasAdditionalItems         bool
	AdditionalItems            *GenSchema
	Object                     *GenSchema
	XMLName                    string
	CustomTag                  string
	Properties                 GenSchemaList
	AllOf                      GenSchemaList
	HasAdditionalProperties    bool
	IsAdditionalProperties     bool
	AdditionalProperties       *GenSchema
	StrictAdditionalProperties bool
	ReadOnly                   bool
	IsBaseType                 bool
	HasBaseType                bool
	IsSubType                  bool
	IsExported                 bool
	DiscriminatorField         string
	DiscriminatorValue         string
	Discriminates              map[string]string
	Parents                    []string
	Default                    interface{}
	ExternalDocs               *spec.ExternalDocumentation
}

func (g GenSchemaList) Len() int      { return len(g) }
func (g GenSchemaList) Swap(i, j int) { g[i], g[j] = g[j], g[i] }
func (g GenSchemaList) Less(i, j int) bool {
	a, okA := g[i].Extensions[xOrder].(float64)
	b, okB := g[j].Extensions[xOrder].(float64)

	// If both properties have x-order defined, then the one with lower x-order is smaller
	if okA && okB {
		return a < b
	}

	// If only the first property has x-order defined, then it is smaller
	if okA {
		return true
	}

	// If only the second property has x-order defined, then it is smaller
	if okB {
		return false
	}

	// If neither property has x-order defined, then the one with lower lexicographic name is smaller
	return g[i].Name < g[j].Name
}

type sharedValidations struct {
	HasValidations bool
	Required       bool

	// String validations
	MaxLength *int64
	MinLength *int64
	Pattern   string

	// Number validations
	MultipleOf       *float64
	Minimum          *float64
	Maximum          *float64
	ExclusiveMinimum bool
	ExclusiveMaximum bool

	Enum      []interface{}
	ItemsEnum []interface{}

	// Slice validations
	MinItems            *int64
	MaxItems            *int64
	UniqueItems         bool
	HasSliceValidations bool

	// Not used yet (perhaps intended for maxProperties, minProperties validations?)
	NeedsSize bool

	// NOTE: "patternProperties" and "dependencies" not supported by Swagger 2.0
}

// pruneEnums omit nil from enum values
func (s *sharedValidations) pruneEnums(sg schemaGenContext) {
	if s.Enum == nil {
		return
	}

	var newEnums []interface{}
	containsNil := false
	containsComplex := false
	for _, enumValue := range s.Enum {
		if enumValue != nil {
			switch enumValue.(type) {
			// bool, string, number(int, float)
			case bool, string, int, float64, float32:
				newEnums = append(newEnums, enumValue)
			default:
				containsComplex = true
			}
		} else {
			containsNil = true
		}
	}
	if containsComplex || containsNil {
		modelName := sg.Path
		if sg.Container != "" {
			modelName = fmt.Sprintf("%s.%s", sg.Container, modelName)
		}
		if containsNil {
			s.Enum = newEnums
			log.Printf("[WARN] enum values in model <%s> contains nil value and the nil value is omitted by KCL", modelName)
		}
		if containsComplex {
			log.Fatalf("enum values in model <%s> contains complex value type which is forbidden in KCL", modelName)
		}
	}
}

// GenApp represents all the meta data needed to generate an application
// from a swagger spec
type GenApp struct {
	GenCommon
	Package      string
	BasePath     string
	Info         *spec.Info
	ExternalDocs *spec.ExternalDocumentation
	Models       []GenDefinition
	GenOpts      *GenOpts
}

// UseGoStructFlags returns true when no strategy is specified or it is set to "go-flags"
func (g *GenApp) UseGoStructFlags() bool {
	if g.GenOpts == nil {
		return true
	}
	return g.GenOpts.FlagStrategy == "" || g.GenOpts.FlagStrategy == "go-flags"
}

// UsePFlags returns true when the flag strategy is set to pflag
func (g *GenApp) UsePFlags() bool {
	return g.GenOpts != nil && strings.HasPrefix(g.GenOpts.FlagStrategy, "pflag")
}

// UseFlags returns true when the flag strategy is set to flag
func (g *GenApp) UseFlags() bool {
	return g.GenOpts != nil && strings.HasPrefix(g.GenOpts.FlagStrategy, "flag")
}

// UseIntermediateMode for https://wiki.mozilla.org/Security/Server_Side_TLS#Intermediate_compatibility_.28default.29
func (g *GenApp) UseIntermediateMode() bool {
	return g.GenOpts != nil && g.GenOpts.CompatibilityMode == "intermediate"
}

// UseModernMode for https://wiki.mozilla.org/Security/Server_Side_TLS#Modern_compatibility
func (g *GenApp) UseModernMode() bool {
	return g.GenOpts == nil || g.GenOpts.CompatibilityMode == "" || g.GenOpts.CompatibilityMode == "modern"
}

// GenSerGroups sorted representation of serializer groups
type GenSerGroups []GenSerGroup

func (g GenSerGroups) Len() int           { return len(g) }
func (g GenSerGroups) Swap(i, j int)      { g[i], g[j] = g[j], g[i] }
func (g GenSerGroups) Less(i, j int) bool { return g[i].Name < g[j].Name }

// GenSerGroup represents a group of serializers: this links a serializer to a list of
// prioritized media types (mime).
type GenSerGroup struct {
	GenSerializer

	// All media types for this serializer. The redundant representation allows for easier use in templates
	AllSerializers GenSerializers
}

// GenSerializers sorted representation of serializers
type GenSerializers []GenSerializer

func (g GenSerializers) Len() int           { return len(g) }
func (g GenSerializers) Swap(i, j int)      { g[i], g[j] = g[j], g[i] }
func (g GenSerializers) Less(i, j int) bool { return g[i].MediaType < g[j].MediaType }

// GenSerializer represents a single serializer for a particular media type
type GenSerializer struct {
	AppName        string // Application name
	ReceiverName   string
	Name           string   // Name of the Producer/Consumer (e.g. json, yaml, txt, bin)
	MediaType      string   // mime
	Implementation string   // func implementing the Producer/Consumer
	Parameters     []string // parameters supported by this serializer
}

// GenSecurityScheme represents a security scheme for code generation
type GenSecurityScheme struct {
	AppName      string
	ID           string
	Name         string
	ReceiverName string
	IsBasicAuth  bool
	IsAPIKeyAuth bool
	IsOAuth2     bool
	Scopes       []string
	Source       string
	// from spec.SecurityScheme
	Description      string
	Type             string
	In               string
	Flow             string
	AuthorizationURL string
	TokenURL         string
	Extensions       map[string]interface{}
}

// GenSecuritySchemes sorted representation of serializers
type GenSecuritySchemes []GenSecurityScheme

func (g GenSecuritySchemes) Len() int           { return len(g) }
func (g GenSecuritySchemes) Swap(i, j int)      { g[i], g[j] = g[j], g[i] }
func (g GenSecuritySchemes) Less(i, j int) bool { return g[i].ID < g[j].ID }

// GenSecurityRequirement represents a security requirement for an operation
type GenSecurityRequirement struct {
	Name   string
	Scopes []string
}

// GenSecurityRequirements represents a compounded security requirement specification.
// In a []GenSecurityRequirements complete requirements specification,
// outer elements are interpreted as optional requirements (OR), and
// inner elements are interpreted as jointly required (AND).
type GenSecurityRequirements []GenSecurityRequirement

func (g GenSecurityRequirements) Len() int           { return len(g) }
func (g GenSecurityRequirements) Swap(i, j int)      { g[i], g[j] = g[j], g[i] }
func (g GenSecurityRequirements) Less(i, j int) bool { return g[i].Name < g[j].Name }
