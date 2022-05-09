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
	"github.com/go-openapi/analysis"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
)

type discInfo struct {
	Discriminators map[string]discor
	Discriminated  map[string]discee
}

type discor struct {
	FieldName string   `json:"fieldName"`
	KclType   string   `json:"kclType"`
	JSONName  string   `json:"jsonName"`
	Children  []discee `json:"children"`
}

type discee struct {
	FieldName  string   `json:"fieldName"`
	FieldValue string   `json:"fieldValue"`
	KclType    string   `json:"kclType"`
	JSONName   string   `json:"jsonName"`
	Ref        spec.Ref `json:"ref"`
	ParentRef  spec.Ref `json:"parentRef"`
}

func discriminatorInfo(doc *analysis.Spec) *discInfo {
	baseTypes := make(map[string]discor)
	for _, sch := range doc.AllDefinitions() {
		if sch.Schema.Discriminator != "" {
			tpe, _ := sch.Schema.Extensions.GetString(xKclName)
			if tpe == "" {
				tpe = swag.ToGoName(sch.Name)
			}
			baseTypes[sch.Ref.String()] = discor{
				FieldName: sch.Schema.Discriminator,
				KclType:   tpe,
				JSONName:  sch.Name,
			}
		}
	}

	subTypes := make(map[string]discee)
	for _, sch := range doc.SchemasWithAllOf() {
		for _, ao := range sch.Schema.AllOf {
			if ao.Ref.String() != "" {
				if bt, ok := baseTypes[ao.Ref.String()]; ok {
					name, _ := sch.Schema.Extensions.GetString(xSchema)
					if name == "" {
						name = sch.Name
					}
					tpe, _ := sch.Schema.Extensions.GetString(xKclName)
					if tpe == "" {
						tpe = swag.ToGoName(sch.Name)
					}
					dce := discee{
						FieldName:  bt.FieldName,
						FieldValue: name,
						Ref:        sch.Ref,
						ParentRef:  ao.Ref,
						JSONName:   sch.Name,
						KclType:    tpe,
					}
					subTypes[sch.Ref.String()] = dce
					bt.Children = append(bt.Children, dce)
					baseTypes[ao.Ref.String()] = bt
				}
			}
		}
	}
	return &discInfo{Discriminators: baseTypes, Discriminated: subTypes}
}
