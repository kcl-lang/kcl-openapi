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
	"testing"

	"github.com/go-openapi/spec"
)

// objectMetaFixture builds an isolated spec containing every bundled k8s
// definition (so candidate $refs resolve) with the ObjectMeta definition
// replaced by a CRD-style copy under candidateKey, optionally mutated.
func objectMetaFixture(t *testing.T, candidateKey string, mutate func(*spec.Schema)) spec.Definitions {
	t.Helper()
	k8sSwagger, err := bundledK8sSpec()
	if err != nil {
		t.Fatalf("load bundled k8s spec: %v", err)
	}
	const objectMetaDef = "k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"
	defs := make(spec.Definitions, len(k8sSwagger.Definitions))
	for k, v := range k8sSwagger.Definitions {
		defs[k] = deepCopySchema(t, v)
	}
	orig, ok := defs[objectMetaDef]
	if !ok {
		t.Fatalf("%s not found in bundled k8s spec", objectMetaDef)
	}
	cp := deepCopySchema(t, orig)
	delete(cp.Extensions, xKclType)
	cp.Description = "stripped copy"
	if mutate != nil {
		mutate(&cp)
	}
	delete(defs, objectMetaDef)
	defs[candidateKey] = cp
	return defs
}

func deepCopySchema(t *testing.T, s spec.Schema) spec.Schema {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var cp spec.Schema
	if err := json.Unmarshal(raw, &cp); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	return cp
}

func TestDedupeCrdDefsAgainstK8s(t *testing.T) {
	defs := objectMetaFixture(t, "WorkflowObjectMeta", nil)
	// a divergent copy drops a property
	divergent := deepCopySchema(t, defs["WorkflowObjectMeta"])
	delete(divergent.Properties, "name")
	defs["LegacyObjectMeta"] = divergent

	sw := spec.Swagger{SwaggerProps: spec.SwaggerProps{Definitions: defs}}
	n := DedupeCrdDefsAgainstK8s(&sw, map[string]struct{}{
		"WorkflowObjectMeta": {},
		"LegacyObjectMeta":   {},
	})
	if n != 1 {
		t.Fatalf("aliased %d definitions, want 1", n)
	}

	aliased := sw.Definitions["WorkflowObjectMeta"]
	if len(aliased.Properties) != 0 {
		t.Errorf("aliased definition keeps local properties: %+v", aliased.Properties)
	}
	xt, ok := aliased.Extensions[xKclType].(map[string]interface{})
	if !ok {
		t.Fatalf("aliased definition lacks %s extension: %+v", xKclType, aliased.Extensions)
	}
	if xt["type"] != "ObjectMeta" {
		t.Errorf("alias type = %v, want ObjectMeta", xt["type"])
	}
	imp, ok := xt["import"].(map[string]interface{})
	if !ok || imp["package"] != "k8s.apimachinery.pkg.apis.meta.v1.object_meta" {
		t.Errorf("alias import = %+v", xt["import"])
	}

	if got := sw.Definitions["LegacyObjectMeta"]; len(got.Properties) == 0 {
		t.Errorf("divergent definition must stay local, got alias %+v", got.Extensions)
	}
}

func TestDedupeCrdDefsAgainstK8sRequiredMismatch(t *testing.T) {
	// required constraints affect the generated KCL: they must be compared.
	defs := objectMetaFixture(t, "CrdObjectMeta", func(s *spec.Schema) {
		s.Required = []string{"name"}
	})
	sw := spec.Swagger{SwaggerProps: spec.SwaggerProps{Definitions: defs}}
	if n := DedupeCrdDefsAgainstK8s(&sw, map[string]struct{}{"CrdObjectMeta": {}}); n != 0 {
		t.Fatalf("required mismatch must not alias, aliased %d", n)
	}
}

func TestDedupeCrdDefsAgainstK8sNoCandidates(t *testing.T) {
	defs := objectMetaFixture(t, "CrdObjectMeta", nil)
	sw := spec.Swagger{SwaggerProps: spec.SwaggerProps{Definitions: defs}}
	if n := DedupeCrdDefsAgainstK8s(&sw, nil); n != 0 {
		t.Fatalf("aliased %d without candidates", n)
	}
	// candidates not present in the spec are skipped
	if n := DedupeCrdDefsAgainstK8s(&sw, map[string]struct{}{"Nope": {}}); n != 0 {
		t.Fatalf("aliased %d for unknown candidates", n)
	}
}
