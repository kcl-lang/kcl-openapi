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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-openapi/spec"

	"kcl-lang.io/kcl-openapi/pkg/kube_resource/generator/assets/static"
)

// k8sSpecAsset is the embedded swagger of the bundled kubernetes types.
const k8sSpecAsset = "api_spec/k8s/k8s.json"

// extDedupeAlias marks definitions that were replaced by an x-kcl-type
// alias because they duplicate a bundled k8s type (see DedupeCrdDefsAgainstK8s).
// Unlike x-kcl-type definitions written by a spec author — which still
// render as local files — these render no file: the type is imported from
// the external package instead.
const extDedupeAlias = "x-kcl-dedupe-alias"

// DedupeCrdDefsAgainstK8s replaces CRD-generated definitions that are
// structurally identical to a bundled k8s type (e.g. a PodSpec inlined by a
// third-party CRD) with an x-kcl-type alias pointing at the official k8s
// module. References then render as `import k8s.<...>` + `<alias>.<Type>`
// instead of a duplicated local schema, which massively shrinks CRDs that
// embed kubernetes types (argo-workflows, tekton, ...).
//
// candidates is the set of definition names to consider (typically the
// hoisted definitions produced by ShortenCrdDefinitionNames). The number of
// aliased definitions is returned.
func DedupeCrdDefsAgainstK8s(sw *spec.Swagger, candidates map[string]struct{}) int {
	defs := sw.Definitions
	if len(defs) == 0 || len(candidates) == 0 {
		return 0
	}
	// The bundled k8s types that are actually referenced by the spec get
	// bundled by flattening, but most of them (PodSpec, Affinity, ...) stay
	// in the asset file because nothing refers to them: build the match
	// index from the full embedded k8s spec.
	k8sIndex, err := bundledK8sIndex()
	if err != nil || len(k8sIndex) == 0 {
		return 0
	}

	aliased := 0
	names := make([]string, 0, len(candidates))
	for name := range candidates {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sch, ok := defs[name]
		if !ok {
			continue
		}
		if _, isAlias := sch.Extensions[xKclType]; isAlias {
			continue // already an alias (or a bundled k8s type)
		}
		h := canonicalSchemaString(sw, sch)
		matches := k8sIndex[h]
		if len(matches) == 0 {
			continue
		}
		// Several k8s API versions may share one canonical form (e.g.
		// rbac.v1.Subject and rbac.v1beta1.Subject). The embedded k8s spec
		// predates the official k8s module releases, so deprecated API paths
		// may no longer exist there: prefer the shortest (typically the
		// stable) import path, then the lexicographically smallest, so the
		// pick is deterministic.
		sort.Slice(matches, func(i, j int) bool {
			if len(matches[i].pkg) != len(matches[j].pkg) {
				return len(matches[i].pkg) < len(matches[j].pkg)
			}
			if matches[i].pkg != matches[j].pkg {
				return matches[i].pkg < matches[j].pkg
			}
			return matches[i].typeName < matches[j].typeName
		})
		alias := spec.Schema{}
		alias.AddExtension(xKclType, matches[0].ext)
		alias.AddExtension(extDedupeAlias, true)
		defs[name] = alias
		aliased++
	}
	return aliased
}

// k8sAliasInfo carries what an aliased definition needs: the x-kcl-type
// extension value pointing at the external k8s package.
type k8sAliasInfo struct {
	typeName string
	pkg      string // import package from the x-kcl-type extension
	ext      interface{}
}

var (
	k8sIndexOnce sync.Once
	k8sIndexVal  map[string][]k8sAliasInfo
	k8sIndexErr  error
	k8sSpecOnce  sync.Once
	k8sSpecVal   *spec.Swagger
	k8sSpecErr   error
)

// bundledK8sSpec parses the embedded full kubernetes swagger exactly once
// per process.
func bundledK8sSpec() (*spec.Swagger, error) {
	k8sSpecOnce.Do(func() {
		raw, ok := static.Files[k8sSpecAsset]
		if !ok {
			k8sSpecErr = fmt.Errorf("embedded asset %s not found", k8sSpecAsset)
			return
		}
		var sw spec.Swagger
		if err := json.Unmarshal([]byte(raw), &sw); err != nil {
			k8sSpecErr = fmt.Errorf("parse embedded %s: %w", k8sSpecAsset, err)
			return
		}
		k8sSpecVal = &sw
	})
	return k8sSpecVal, k8sSpecErr
}

// bundledK8sIndex canonicalizes every definition of the embedded k8s spec
// exactly once per process.
func bundledK8sIndex() (map[string][]k8sAliasInfo, error) {
	k8sIndexOnce.Do(func() {
		k8sSwagger, err := bundledK8sSpec()
		if err != nil {
			k8sIndexErr = err
			return
		}
		idx := make(map[string][]k8sAliasInfo, len(k8sSwagger.Definitions))
		for _, sch := range k8sSwagger.Definitions {
			v, ok := sch.Extensions[xKclType]
			if !ok {
				continue
			}
			xt, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			typeName, _ := xt["type"].(string)
			pkg := ""
			if imp, ok := xt["import"].(map[string]interface{}); ok {
				pkg, _ = imp["package"].(string)
			}
			h := canonicalSchemaString(k8sSwagger, sch)
			idx[h] = append(idx[h], k8sAliasInfo{typeName: typeName, pkg: pkg, ext: v})
		}
		k8sIndexVal = idx
	})
	return k8sIndexVal, k8sIndexErr
}

// canonicalSchemaString renders a schema as a deterministic string that
// captures everything affecting the generated KCL: types, constraints,
// composition and the x-kubernetes-* extensions. Cosmetic fields
// (description, title, examples, ordering annotations, ...) are dropped, and
// local $refs are resolved recursively, so two schemas that differ only in
// prose or in how deeply they were inlined still compare equal.
func canonicalSchemaString(sw *spec.Swagger, s spec.Schema) string {
	var sb strings.Builder
	writeCanonicalSchema(&sb, sw, s, make(map[string]bool))
	return sb.String()
}

func writeCanonicalSchema(sb *strings.Builder, sw *spec.Swagger, s spec.Schema, visiting map[string]bool) {
	if s.Ref.String() != "" {
		name := refDefinitionName(s.Ref)
		if target, ok := sw.Definitions[name]; ok && !visiting[name] {
			visiting[name] = true
			writeCanonicalSchema(sb, sw, target, visiting)
			delete(visiting, name)
			return
		}
		// Reference to a schema outside the spec or a cycle: keep a stable
		// marker. CRD and k8s types are plain trees, so this rarely fires.
		sb.WriteString("&ref(")
		sb.WriteString(name)
		sb.WriteByte(')')
		return
	}

	sb.WriteByte('{')
	writeStrProp := func(key, value string) {
		if value != "" {
			sb.WriteString(key)
			sb.WriteByte('=')
			sb.WriteString(value)
			sb.WriteByte(';')
		}
	}
	writeFloatProp := func(key string, v *float64) {
		if v != nil {
			writeStrProp(key, strconv.FormatFloat(*v, 'g', -1, 64))
		}
	}
	writeIntProp := func(key string, v *int64) {
		if v != nil {
			writeStrProp(key, strconv.FormatInt(*v, 10))
		}
	}

	writeStrProp("t", strings.Join(s.Type, ","))
	writeStrProp("f", s.Format)
	if s.Default != nil {
		sb.WriteString("d=")
		sb.WriteString(canonicalValueString(s.Default))
		sb.WriteByte(';')
	}
	writeFloatProp("max", s.Maximum)
	writeFloatProp("min", s.Minimum)
	if s.ExclusiveMaximum {
		sb.WriteString("xmax=1;")
	}
	if s.ExclusiveMinimum {
		sb.WriteString("xmin=1;")
	}
	writeFloatProp("m", s.MultipleOf)
	writeIntProp("ml", s.MaxLength)
	writeIntProp("nl", s.MinLength)
	writeStrProp("p", s.Pattern)
	writeIntProp("mi", s.MaxItems)
	writeIntProp("ni", s.MinItems)
	if s.UniqueItems {
		sb.WriteString("ui=1;")
	}
	if len(s.Required) > 0 {
		required := append([]string(nil), s.Required...)
		sort.Strings(required)
		writeStrProp("r", strings.Join(required, ","))
	}
	if len(s.Enum) > 0 {
		parts := make([]string, 0, len(s.Enum))
		for _, e := range s.Enum {
			parts = append(parts, canonicalValueString(e))
		}
		sort.Strings(parts)
		writeStrProp("e", strings.Join(parts, ","))
	}

	if s.Items != nil {
		if s.Items.Schema != nil {
			sb.WriteString("items:")
			writeCanonicalSchema(sb, sw, *s.Items.Schema, visiting)
			sb.WriteByte(';')
		}
		for i := range s.Items.Schemas {
			sb.WriteString("tuple:")
			writeCanonicalSchema(sb, sw, s.Items.Schemas[i], visiting)
			sb.WriteByte(';')
		}
	}
	writeSchemaMap := func(key string, m map[string]spec.Schema) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(key)
			sb.WriteByte('[')
			sb.WriteString(k)
			sb.WriteString("]=")
			writeCanonicalSchema(sb, sw, m[k], visiting)
			sb.WriteByte(';')
		}
	}
	writeSchemaMap("prop", s.Properties)
	writeSchemaMap("pat", s.PatternProperties)
	if s.AdditionalProperties != nil {
		sb.WriteString("addl=")
		if s.AdditionalProperties.Schema != nil {
			writeCanonicalSchema(sb, sw, *s.AdditionalProperties.Schema, visiting)
		} else if s.AdditionalProperties.Allows {
			sb.WriteByte('1')
		} else {
			sb.WriteByte('0')
		}
		sb.WriteByte(';')
	}
	for i := range s.AllOf {
		sb.WriteString("allOf:")
		writeCanonicalSchema(sb, sw, s.AllOf[i], visiting)
		sb.WriteByte(';')
	}
	for i := range s.AnyOf {
		sb.WriteString("anyOf:")
		writeCanonicalSchema(sb, sw, s.AnyOf[i], visiting)
		sb.WriteByte(';')
	}
	for i := range s.OneOf {
		sb.WriteString("oneOf:")
		writeCanonicalSchema(sb, sw, s.OneOf[i], visiting)
		sb.WriteByte(';')
	}
	if s.Not != nil {
		sb.WriteString("not:")
		writeCanonicalSchema(sb, sw, *s.Not, visiting)
		sb.WriteByte(';')
	}
	writeStrProp("disc", s.Discriminator)

	// x-kubernetes-* extensions drive the generated KCL (int-or-string
	// unions, CEL validations, list semantics); include them. Other
	// extensions (x-order, x-kcl-*, ...) never affect the structure.
	var xkeys []string
	for k := range s.Extensions {
		if strings.HasPrefix(k, "x-kubernetes-") {
			xkeys = append(xkeys, k)
		}
	}
	sort.Strings(xkeys)
	for _, k := range xkeys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(canonicalValueString(s.Extensions[k]))
		sb.WriteByte(';')
	}
	sb.WriteByte('}')
}

// canonicalValueString renders an extension/default value deterministically.
func canonicalValueString(v interface{}) string {
	switch tv := v.(type) {
	case nil:
		return "null"
	case string:
		return "s:" + tv
	case bool:
		if tv {
			return "b:true"
		}
		return "b:false"
	case float64:
		return "n:" + strconv.FormatFloat(tv, 'g', -1, 64)
	case []interface{}:
		parts := make([]string, 0, len(tv))
		for _, item := range tv {
			parts = append(parts, canonicalValueString(item))
		}
		sort.Strings(parts)
		return "[" + strings.Join(parts, ",") + "]"
	case map[string]interface{}:
		keys := make([]string, 0, len(tv))
		for k := range tv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		sb.WriteByte('{')
		for _, k := range keys {
			sb.WriteString(k)
			sb.WriteByte(':')
			sb.WriteString(canonicalValueString(tv[k]))
			sb.WriteByte(';')
		}
		sb.WriteByte('}')
		return sb.String()
	default:
		return "?"
	}
}

// refDefinitionName extracts the definition name from a local
// "#/definitions/<name>" ref.
func refDefinitionName(ref spec.Ref) string {
	u := ref.GetURL()
	if u.Scheme != "" || u.Host != "" || u.Path != "" {
		return ""
	}
	const prefix = "/definitions/"
	if !strings.HasPrefix(u.Fragment, prefix) {
		return ""
	}
	return u.Fragment[len(prefix):]
}
