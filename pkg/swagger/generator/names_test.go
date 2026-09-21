// Copyright 2024 The KCL Authors
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
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/go-openapi/spec"
)

func crdRootSchema() spec.Schema {
	s := spec.Schema{}
	s.AddExtension(extCrdRoot, true)
	return s
}

// rootWithMarker adds the CRD-root marker to an existing schema (roots keep
// their properties).
func rootWithMarker(s spec.Schema) spec.Schema {
	s.AddExtension(extCrdRoot, true)
	return s
}

func refProperty(name, ref string) spec.Schema {
	s := spec.Schema{}
	s.SetProperty(name, *spec.RefSchema(ref))
	return s
}

// refString renders a schema's $ref (Ref.String has a pointer receiver and
// map-indexed schemas are not addressable).
func refString(s spec.Schema) string {
	r := s.Ref
	return r.String()
}

func TestShortenCrdDefinitionNames(t *testing.T) {
	const (
		root      = "argoproj.io.v1alpha1.ClusterWorkflowTemplate"
		nested1   = root + ".spec.affinity.nodeAffinity"
		nested2   = root + ".spec.templates.items.0"
		nested3   = nested1 + ".requiredDuringSchedulingIgnoredDuringExecution.items.0"
		k8sDef    = "k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"
		k8sDef2   = "k8s.apimachinery.pkg.apis.meta.v1.OwnerReference"
		unrelated = "io.k8s.api.core.v1.PodSpec" // no marker, not under a CRD root
	)
	sw := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Definitions: spec.Definitions{
				// property order deliberately reversed vs. sorted order
				nested3:   refProperty("preference", "#/definitions/"+nested1),
				nested2:   refProperty("affinity", "#/definitions/"+nested1),
				nested1:   refProperty("metadata", "#/definitions/"+k8sDef),
				root:      rootWithMarker(refProperty("spec", "#/definitions/"+nested1)),
				k8sDef:    refProperty("ownerReferences", "#/definitions/"+k8sDef2),
				k8sDef2:   spec.Schema{},
				unrelated: spec.Schema{},
			},
		},
	}

	mapping, _ := ShortenCrdDefinitionNames(&sw)
	if mapping == nil {
		t.Fatal("expected a rename mapping")
	}

	want := map[string]string{
		root:    "ClusterWorkflowTemplate",
		nested1: "ClusterWorkflowTemplateAffinityNodeAffinity",
		nested2: "ClusterWorkflowTemplateSpecTemplates",
		nested3: "ClusterWorkflowTemplateNodeAffinityRequiredDuringSchedulingIgnoredDuringExecution",
	}
	for old, wantNew := range want {
		if mapping[old] != wantNew {
			t.Errorf("mapping[%q] = %q, want %q", old, mapping[old], wantNew)
		}
	}
	for _, untouched := range []string{k8sDef, k8sDef2, unrelated} {
		if _, ok := mapping[untouched]; ok {
			t.Errorf("non-CRD definition %q must not be renamed (got %q)", untouched, mapping[untouched])
		}
		if _, ok := sw.Definitions[untouched]; !ok {
			t.Errorf("non-CRD definition %q must be kept", untouched)
		}
	}

	// definition keys renamed
	for _, newName := range want {
		if _, ok := sw.Definitions[newName]; !ok {
			t.Errorf("definition %q not found after rename", newName)
		}
	}
	if _, ok := sw.Definitions[root]; ok {
		t.Errorf("old definition key %q still present", root)
	}

	// local $refs rewritten, k8s refs untouched
	rootDef := sw.Definitions["ClusterWorkflowTemplate"]
	if got := refString(rootDef.Properties["spec"]); got != "#/definitions/ClusterWorkflowTemplateAffinityNodeAffinity" {
		t.Errorf("root spec ref = %q", got)
	}
	nested1Def := sw.Definitions["ClusterWorkflowTemplateAffinityNodeAffinity"]
	if got := refString(nested1Def.Properties["metadata"]); got != "#/definitions/"+k8sDef {
		t.Errorf("k8s ref must stay untouched, got %q", got)
	}
	nested3Def := sw.Definitions["ClusterWorkflowTemplateNodeAffinityRequiredDuringSchedulingIgnoredDuringExecution"]
	if got := refString(nested3Def.Properties["preference"]); got != "#/definitions/ClusterWorkflowTemplateAffinityNodeAffinity" {
		t.Errorf("nested ref = %q", got)
	}
}

func TestShortenCrdDefinitionNamesMultiVersion(t *testing.T) {
	const (
		rootV1alpha1 = "core.oam.dev.v1alpha2.ContainerizedWorkload"
		rootV1beta1  = "core.oam.dev.v1beta1.ContainerizedWorkload"
	)
	sw := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Definitions: spec.Definitions{
				rootV1alpha1:                        crdRootSchema(),
				rootV1beta1:                         crdRootSchema(),
				rootV1alpha1 + ".spec":              spec.Schema{},
				rootV1beta1 + ".spec.restartPolicy": spec.Schema{},
			},
		},
	}
	mapping, _ := ShortenCrdDefinitionNames(&sw)
	if got := mapping[rootV1alpha1]; got != "ContainerizedWorkloadV1alpha2" {
		t.Errorf("v1alpha2 root = %q", got)
	}
	if got := mapping[rootV1beta1]; got != "ContainerizedWorkloadV1beta1" {
		t.Errorf("v1beta1 root = %q", got)
	}
	if got := mapping[rootV1alpha1+".spec"]; got != "ContainerizedWorkloadV1alpha2Spec" {
		t.Errorf("v1alpha2 spec = %q", got)
	}
	if got := mapping[rootV1beta1+".spec.restartPolicy"]; got != "ContainerizedWorkloadV1beta1SpecRestartPolicy" {
		t.Errorf("v1beta1 restartPolicy = %q", got)
	}
}

func TestShortenCrdDefinitionNamesCollision(t *testing.T) {
	const root = "example.io.v1.Widget"
	sw := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Definitions: spec.Definitions{
				root:                   crdRootSchema(),
				root + ".spec.fooBar":  spec.Schema{},
				root + ".spec.foo_bar": spec.Schema{},
				root + ".status":       spec.Schema{},
			},
		},
	}
	mapping, _ := ShortenCrdDefinitionNames(&sw)
	// "fooBar" sorts before "foo_bar"; both pascalize to FooBar, so the
	// latter falls back to a numeric suffix on the full path.
	if got := mapping[root+".spec.fooBar"]; got != "WidgetSpecFooBar" {
		t.Errorf("spec.fooBar = %q", got)
	}
	if got := mapping[root+".spec.foo_bar"]; got != "WidgetSpecFooBar2" {
		t.Errorf("spec.foo_bar = %q", got)
	}
	names := map[string]bool{}
	for _, n := range mapping {
		if names[n] {
			t.Errorf("duplicate name %q in mapping %v", n, mapping)
		}
		names[n] = true
	}
}

func TestShortenCrdDefinitionNamesKeepsXkclName(t *testing.T) {
	const root = "example.io.v1.Widget"
	custom := spec.Schema{}
	custom.AddExtension(xKclName, "MyCustom")
	sw := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Definitions: spec.Definitions{
				root:                 crdRootSchema(),
				root + ".spec":       custom,
				root + ".spec.items": spec.Schema{},
			},
		},
	}
	mapping, _ := ShortenCrdDefinitionNames(&sw)
	if _, ok := mapping[root+".spec"]; ok {
		t.Errorf("definition with %s must not be renamed", xKclName)
	}
	// the definition key is untouched: the generator renders the custom
	// name from the extension at codegen time.
	if _, ok := sw.Definitions[root+".spec"]; !ok {
		t.Error("definition with x-kcl-name must keep its original key")
	}
	// "items" is a noise segment, so the only meaningful path segment is
	// "spec"; the parent carries x-kcl-name and is skipped, leaving the
	// shortened name free to claim "WidgetSpec".
	if got := mapping[root+".spec.items"]; got != "WidgetSpec" {
		t.Errorf("spec.items = %q", got)
	}
}

func TestShortenCrdDefinitionNamesNoRoots(t *testing.T) {
	sw := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Definitions: spec.Definitions{
				"io.k8s.api.core.v1.PodSpec": spec.Schema{},
			},
		},
	}
	if mapping, _ := ShortenCrdDefinitionNames(&sw); mapping != nil {
		t.Errorf("expected nil mapping without CRD roots, got %v", mapping)
	}
	if _, ok := sw.Definitions["io.k8s.api.core.v1.PodSpec"]; !ok {
		t.Error("definitions must be untouched")
	}
}

func TestShortenCrdDefinitionNamesDeterministic(t *testing.T) {
	build := func() *spec.Swagger {
		return &spec.Swagger{
			SwaggerProps: spec.SwaggerProps{
				Definitions: spec.Definitions{
					"argoproj.io.v1alpha1.ClusterWorkflowTemplate":                     crdRootSchema(),
					"argoproj.io.v1alpha1.ClusterWorkflowTemplate.spec.affinity":       spec.Schema{},
					"argoproj.io.v1alpha1.ClusterWorkflowTemplate.spec.tolerations":    spec.Schema{},
					"argoproj.io.v1alpha1.ClusterWorkflowTemplate.spec.affinity.items": spec.Schema{},
				},
			},
		}
	}
	var first map[string]string
	for i := 0; i < 50; i++ {
		m, _ := ShortenCrdDefinitionNames(build())
		if first == nil {
			first = m
			continue
		}
		if !reflect.DeepEqual(first, m) {
			b1, _ := json.Marshal(first)
			b2, _ := json.Marshal(m)
			t.Fatalf("non-deterministic mapping:\n%s\n%s", b1, b2)
		}
	}
}

func TestShortenCrdDefinitionNamesNonLocalRefsUntouched(t *testing.T) {
	const root = "example.io.v1.Widget"
	ext := spec.Schema{}
	ext.SetProperty("out", *spec.RefSchema("other.json#/definitions/External"))
	sw := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Definitions: spec.Definitions{
				root:           crdRootSchema(),
				root + ".spec": ext,
			},
		},
	}
	ShortenCrdDefinitionNames(&sw)
	got := refString(sw.Definitions["WidgetSpec"].Properties["out"])
	if got != "other.json#/definitions/External" {
		t.Errorf("non-local ref rewritten: %q", got)
	}
}

func TestShortenCrdDefinitionNamesHoistsInlineObjects(t *testing.T) {
	const root = "argoproj.io.v1alpha1.ClusterWorkflowTemplate"

	inlineObj := func(props map[string]spec.Schema) spec.Schema {
		return spec.Schema{SchemaProps: spec.SchemaProps{Properties: props}}
	}
	scalar := spec.Schema{}
	arrayOf := func(item spec.Schema) spec.Schema {
		return spec.Schema{SchemaProps: spec.SchemaProps{
			Type:  []string{"array"},
			Items: &spec.SchemaOrArray{Schema: &item},
		}}
	}

	nodeAffinity := inlineObj(map[string]spec.Schema{"requiredDuringSchedulingIgnoredDuringExecution": scalar})
	affinity := inlineObj(map[string]spec.Schema{"nodeAffinity": nodeAffinity})
	templates := inlineObj(map[string]spec.Schema{
		"metadata": inlineObj(map[string]spec.Schema{"x": scalar}),
	})
	labels := spec.Schema{SchemaProps: spec.SchemaProps{
		Type: []string{"object"},
		AdditionalProperties: &spec.SchemaOrBool{
			Allows: true,
			Schema: &spec.Schema{SchemaProps: spec.SchemaProps{Type: []string{"string"}}},
		},
	}}
	rootSchema := inlineObj(map[string]spec.Schema{
		"spec":      inlineObj(map[string]spec.Schema{"affinity": affinity}),
		"templates": arrayOf(templates),
		"labels":    labels,
	})

	sw := spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Definitions: spec.Definitions{root: rootWithMarker(rootSchema)},
		},
	}
	mapping, _ := ShortenCrdDefinitionNames(&sw)
	if mapping[root] != "ClusterWorkflowTemplate" {
		t.Fatalf("root = %q", mapping[root])
	}

	wantDefs := []string{
		"ClusterWorkflowTemplate",                     // root
		"ClusterWorkflowTemplateSpec",                 // spec
		"ClusterWorkflowTemplateSpecAffinity",         // spec.affinity
		"ClusterWorkflowTemplateAffinityNodeAffinity", // spec.affinity.nodeAffinity
		"ClusterWorkflowTemplateTemplates",            // spec.templates[]
		"ClusterWorkflowTemplateTemplatesMetadata",    // spec.templates[].metadata
	}
	for _, name := range wantDefs {
		if _, ok := sw.Definitions[name]; !ok {
			t.Errorf("hoisted definition %q missing (defs: %v)", name, sortedKeys(sw.Definitions))
		}
	}

	rootDef := sw.Definitions["ClusterWorkflowTemplate"]
	if got := refString(rootDef.Properties["spec"]); got != "#/definitions/ClusterWorkflowTemplateSpec" {
		t.Errorf("spec ref = %q", got)
	}
	specDef := sw.Definitions["ClusterWorkflowTemplateSpec"]
	if got := refString(specDef.Properties["affinity"]); got != "#/definitions/ClusterWorkflowTemplateSpecAffinity" {
		t.Errorf("affinity ref = %q", got)
	}
	templatesProp := rootDef.Properties["templates"]
	if templatesProp.Items == nil || templatesProp.Items.Schema == nil {
		t.Fatalf("templates items missing: %+v", templatesProp)
	}
	if got := refString(*templatesProp.Items.Schema); got != "#/definitions/ClusterWorkflowTemplateTemplates" {
		t.Errorf("templates items ref = %q", got)
	}
	// plain maps of scalars stay inline: no named definition for labels
	for _, name := range sortedKeys(sw.Definitions) {
		if strings.Contains(strings.ToLower(name), "labels") {
			t.Errorf("scalar map must stay inline, got definition %q", name)
		}
	}
	// every generated name is unique
	seen := map[string]bool{}
	for _, name := range sortedKeys(sw.Definitions) {
		if seen[strings.ToLower(name)] {
			t.Errorf("duplicate definition name %q", name)
		}
		seen[strings.ToLower(name)] = true
	}
}
