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
	"fmt"
	"sort"
	"strings"

	"github.com/go-openapi/spec"
	"github.com/go-openapi/swag"
)

// extCrdRoot is the vendor extension set on the root definitions
// (group.version.Kind) produced from a Kubernetes CRD by
// pkg/kube_resource/generator. The same literal is defined there: keep both
// in sync. It marks the definitions the short-name pass may rename — the
// roots themselves and every definition nested below them. Definitions
// without this marker in their ancestry (e.g. the bundled k8s types) always
// keep their original names.
const extCrdRoot = "x-kcl-crd-root"

// extCrdPkg records the package (group.version) a CRD definition belongs to.
// With GenOpts.CrdPackageLayout, definitions carrying this extension render
// into the matching sub-package of the model package.
const extCrdPkg = "x-kcl-crd-pkg"

// shortNameMaxSegments bounds how many nested path segments are kept in a
// shortened definition name. Larger values produce longer but more specific
// names; collisions are still resolved by extending the name towards the
// root, so this is only a preference.
const shortNameMaxSegments = 2

// noiseSegments are path segments contributed by JSON-schema/container
// structure rather than by the API author's property names. They never add
// meaning to a generated schema name.
var noiseSegments = map[string]struct{}{
	"items":                {},
	"properties":           {},
	"definitions":          {},
	"additionalProperties": {},
}

// ShortenCrdDefinitionNames restructures the definitions generated from a
// Kubernetes CRD so that the generated KCL schemas get short, unique,
// deterministic names, and rewrites every local $ref accordingly.
//
// The code generator names anonymous inline objects after the full property
// path that leads to them (e.g. the object at
// spec.affinity.nodeAffinity...matchFields.items[0] inside
// argoproj.io.v1alpha1.ClusterWorkflowTemplate becomes
// ArgoprojIoV1alpha1ClusterWorkflowTemplateSpecAffinityNodeAffinity...
// MatchFieldsItems0). This pass instead:
//
//   - names each root definition (group.version.Kind) after its Kind,
//     appending the version when several versions of the same Kind exist
//     (e.g. ClusterWorkflowTemplateV1alpha1);
//   - hoists every inline complex object below the roots into its own named
//     definition, named rootShort + at most shortNameMaxSegments trailing
//     meaningful path segments, with noise segments such as array
//     "items"/"0" dropped (e.g. ClusterWorkflowTemplateAffinityNodeAffinity);
//   - on collision extends the name towards the root one segment at a time,
//     then de-duplicates with a numeric suffix;
//   - keeps definitions carrying x-kcl-name under their custom name;
//   - leaves definitions not originating from a CRD (e.g. the bundled k8s
//     types) untouched and reserves their names against collisions.
//
// Hoisted definitions become regular models, so each renders as its own file
// with plain cross-references instead of inline anonymous schemas.
//
// The returned maps record the old -> new names and the set of definitions
// hoisted from inline schemas (candidates for DedupeCrdDefsAgainstK8s); both
// are nil when the spec carries no CRD root definitions.
func ShortenCrdDefinitionNames(sw *spec.Swagger) (map[string]string, map[string]struct{}) {
	defs := sw.Definitions
	if len(defs) == 0 {
		return nil, nil
	}

	// Collect the CRD roots, sorted for deterministic output.
	var roots []string
	for name, sch := range defs {
		if _, ok := sch.Extensions[extCrdRoot]; ok {
			roots = append(roots, name)
		}
	}
	if len(roots) == 0 {
		return nil, nil
	}
	sort.Strings(roots)
	isRoot := make(map[string]bool, len(roots))
	for _, r := range roots {
		isRoot[r] = true
	}

	// Map every definition to the CRD root it is nested under, if any. Root
	// names are dot-terminated on purpose so that kinds sharing a prefix
	// (e.g. "Foo" and "FooBar") cannot shadow each other.
	rootOf := make(map[string]string, len(defs))
	for name := range defs {
		if isRoot[name] {
			rootOf[name] = name
			continue
		}
		for _, r := range roots {
			if strings.HasPrefix(name, r+".") {
				rootOf[name] = r
				break
			}
		}
	}

	// Definitions with an explicit x-kcl-name keep it: exclude them from
	// renaming and reserve their custom name.
	used := make(map[string]struct{}, len(defs))
	for name, sch := range defs {
		if nm, ok := sch.Extensions.GetString(xKclName); ok {
			if _, renamed := rootOf[name]; renamed {
				delete(rootOf, name)
				used[strings.ToLower(nm)] = struct{}{}
				continue
			}
		}
		if _, renamed := rootOf[name]; !renamed {
			// Not a CRD definition (e.g. bundled k8s types): keep the name
			// and reserve it so short names never collide with it.
			used[strings.ToLower(name)] = struct{}{}
		}
	}

	mapping := make(map[string]string, len(rootOf))

	// Short root names: Kind, plus the version when several roots share it.
	kindCount := make(map[string]int, len(roots))
	kinds := make(map[string]string, len(roots))
	versions := make(map[string]string, len(roots))
	for _, r := range roots {
		kind, version := splitRootName(r)
		kinds[r] = kind
		versions[r] = version
		kindCount[kind]++
	}
	for _, r := range roots {
		candidate := kinds[r]
		if kindCount[kinds[r]] > 1 {
			candidate += swag.ToGoName(versions[r])
		}
		mapping[r] = claimName(candidate, used)
	}

	// Hoist inline complex objects below each root into named definitions,
	// processing roots in sorted order so names are assigned deterministically.
	hoisted := make(map[string]struct{})
	for _, r := range roots {
		sch := defs[r]
		crdPkg, _ := sch.Extensions.GetString(extCrdPkg)
		hoistInlineSchemas(sw, mapping[r], &sch, nil, used, hoisted, crdPkg)
		defs[r] = sch
	}

	// Short names for any remaining definitions nested under a root by name
	// (e.g. dotted names produced by full flattening), assigned in sorted
	// order so the result does not depend on map iteration.
	nested := make([]string, 0, len(rootOf))
	for name := range rootOf {
		if !isRoot[name] {
			nested = append(nested, name)
		}
	}
	sort.Strings(nested)
	for _, name := range nested {
		mapping[name] = shortNestedName(name, rootOf[name], mapping[rootOf[name]], used)
	}

	// Apply: rename definition keys ...
	renamed := make(spec.Definitions, len(defs))
	for name, sch := range defs {
		if newName, ok := mapping[name]; ok {
			renamed[newName] = sch
		} else {
			renamed[name] = sch
		}
	}
	sw.Definitions = renamed

	// ... and rewrite every local $ref in the spec.
	rewriteDefinitionRefs(sw, mapping)
	return mapping, hoisted
}

// splitRootName splits a CRD root definition name (group.version.Kind) into
// its Kind and version. The group part may contain any number of dots, so the
// version is the segment before the last dot.
func splitRootName(root string) (kind, version string) {
	i := strings.LastIndex(root, ".")
	kind = root[i+1:]
	j := strings.LastIndex(root[:i], ".")
	version = root[j+1 : i]
	return kind, version
}

// isComplexObject reports whether s is an inline object schema worth its own
// definition: it has properties (or composition) and is not already a $ref.
// Pure maps (additionalProperties only) and scalars stay inline.
func isComplexObject(s *spec.Schema) bool {
	return len(s.Properties) > 0 || len(s.AllOf) > 0
}

// hoistInlineSchemas replaces every inline complex object reachable from s
// with a $ref to a freshly named definition, recursing into the hoisted
// definitions. segs carries the meaningful path segments leading to s within
// its CRD root and is used to derive the new names.
func hoistInlineSchemas(sw *spec.Swagger, rootShort string, s *spec.Schema, segs []string, used map[string]struct{}, hoisted map[string]struct{}, crdPkg string) {
	for _, k := range sortedKeys(s.Properties) {
		child := s.Properties[k]
		hoistChild(sw, rootShort, &child, appendSegment(segs, k), used, hoisted, crdPkg)
		s.Properties[k] = child
	}
	for _, k := range sortedKeys(s.PatternProperties) {
		child := s.PatternProperties[k]
		hoistChild(sw, rootShort, &child, appendSegment(segs, k), used, hoisted, crdPkg)
		s.PatternProperties[k] = child
	}
	if s.Items != nil {
		if s.Items.Schema != nil {
			child := *s.Items.Schema
			hoistChild(sw, rootShort, &child, segs, used, hoisted, crdPkg)
			*s.Items.Schema = child
		}
		for i := range s.Items.Schemas {
			hoistChild(sw, rootShort, &s.Items.Schemas[i], segs, used, hoisted, crdPkg)
		}
	}
	if s.AdditionalItems != nil && s.AdditionalItems.Schema != nil {
		child := *s.AdditionalItems.Schema
		hoistChild(sw, rootShort, &child, segs, used, hoisted, crdPkg)
		*s.AdditionalItems.Schema = child
	}
	if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
		child := *s.AdditionalProperties.Schema
		// additionalProperties contributes no name segment of its own: it is
		// structural noise, like array items.
		hoistChild(sw, rootShort, &child, segs, used, hoisted, crdPkg)
		*s.AdditionalProperties.Schema = child
	}
	for i := range s.AllOf {
		hoistChild(sw, rootShort, &s.AllOf[i], segs, used, hoisted, crdPkg)
	}
	for i := range s.AnyOf {
		hoistChild(sw, rootShort, &s.AnyOf[i], segs, used, hoisted, crdPkg)
	}
	for i := range s.OneOf {
		hoistChild(sw, rootShort, &s.OneOf[i], segs, used, hoisted, crdPkg)
	}
	if s.Not != nil {
		hoistChild(sw, rootShort, s.Not, segs, used, hoisted, crdPkg)
	}
}

// hoistChild hoists a single child schema: if it is an inline complex object
// it becomes a named definition (and is recursed into), otherwise the walk
// continues inline. $refs are left untouched.
func hoistChild(sw *spec.Swagger, rootShort string, child *spec.Schema, segs []string, used map[string]struct{}, hoisted map[string]struct{}, crdPkg string) {
	if child == nil {
		return
	}
	if child.Ref.String() != "" {
		return
	}
	if isComplexObject(child) {
		name := claimNestedName(rootShort, segs, used)
		if sw.Definitions == nil {
			sw.Definitions = make(spec.Definitions, 1)
		}
		stored := *child
		if crdPkg != "" {
			stored.AddExtension(extCrdPkg, crdPkg)
		}
		sw.Definitions[name] = stored
		hoisted[name] = struct{}{}
		*child = *spec.RefProperty("#/definitions/" + name)
		cur := sw.Definitions[name]
		hoistInlineSchemas(sw, rootShort, &cur, segs, used, hoisted, crdPkg)
		sw.Definitions[name] = cur
		return
	}
	hoistInlineSchemas(sw, rootShort, child, segs, used, hoisted, crdPkg)
}

func sortedKeys(m map[string]spec.Schema) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func appendSegment(segs []string, seg string) []string {
	out := make([]string, len(segs)+1)
	copy(out, segs)
	out[len(segs)] = seg
	return out
}

// claimNestedName builds and reserves a short name for the inline object at
// the given path segments: the short root name plus at most
// shortNameMaxSegments trailing meaningful segments. On collision the name
// is extended towards the root a few segments at a time; past the extension
// cap it is de-duplicated with a numeric suffix on the shortest candidate,
// so heavily duplicated structures (e.g. a k8s type inlined by a CRD in
// dozens of places) stay short instead of degenerating into full paths.
func claimNestedName(rootShort string, segs []string, used map[string]struct{}) string {
	meaningful := make([]string, 0, len(segs))
	for _, seg := range segs {
		if !isNoiseSegment(seg) {
			meaningful = append(meaningful, seg)
		}
	}
	if len(meaningful) == 0 {
		meaningful = segs
	}

	cap := min(len(meaningful), shortNameMaxSegments+2)
	for n := min(shortNameMaxSegments, len(meaningful)); n <= cap; n++ {
		candidate := rootShort + pascalJoin(meaningful[len(meaningful)-n:])
		if _, taken := used[strings.ToLower(candidate)]; !taken {
			used[strings.ToLower(candidate)] = struct{}{}
			return candidate
		}
	}
	shortest := rootShort + pascalJoin(meaningful[len(meaningful)-min(shortNameMaxSegments, len(meaningful)):])
	return claimName(shortest, used)
}

// shortNestedName builds the short name for a definition whose key is nested
// under a root key (root.path.segments), delegating to claimNestedName.
func shortNestedName(name, root, rootShort string, used map[string]struct{}) string {
	return claimNestedName(rootShort, strings.Split(name[len(root)+1:], "."), used)
}

// claimName reserves name (lower-cased) in used, extending it with a numeric
// suffix until it is unique.
func claimName(name string, used map[string]struct{}) string {
	if _, taken := used[strings.ToLower(name)]; !taken {
		used[strings.ToLower(name)] = struct{}{}
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", name, i)
		if _, taken := used[strings.ToLower(candidate)]; !taken {
			used[strings.ToLower(candidate)] = struct{}{}
			return candidate
		}
	}
}

// pascalJoin camel-cases and concatenates path segments.
func pascalJoin(segments []string) string {
	var sb strings.Builder
	for _, seg := range segments {
		sb.WriteString(swag.ToGoName(seg))
	}
	return sb.String()
}

func isNoiseSegment(seg string) bool {
	if _, ok := noiseSegments[seg]; ok {
		return true
	}
	if seg == "" {
		return false
	}
	for _, r := range seg {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// rewriteDefinitionRefs rewrites every local "#/definitions/<name>" $ref in
// the spec according to mapping.
func rewriteDefinitionRefs(sw *spec.Swagger, mapping map[string]string) {
	for name := range sw.Definitions {
		sch := sw.Definitions[name]
		rewriteSchemaRefs(&sch, mapping)
		sw.Definitions[name] = sch
	}
	if sw.Paths != nil {
		for _, pathItem := range sw.Paths.Paths {
			for i := range pathItem.Parameters {
				rewriteParameterRefs(&pathItem.Parameters[i], mapping)
			}
			for _, op := range []*spec.Operation{
				pathItem.Get, pathItem.Put, pathItem.Post, pathItem.Delete,
				pathItem.Options, pathItem.Head, pathItem.Patch,
			} {
				if op == nil {
					continue
				}
				for i := range op.Parameters {
					rewriteParameterRefs(&op.Parameters[i], mapping)
				}
				if op.Responses == nil {
					continue
				}
				if op.Responses.Default != nil {
					rewriteResponseRefs(op.Responses.Default, mapping)
				}
				for code := range op.Responses.StatusCodeResponses {
					resp := op.Responses.StatusCodeResponses[code]
					rewriteResponseRefs(&resp, mapping)
					op.Responses.StatusCodeResponses[code] = resp
				}
			}
		}
	}
	for name := range sw.Parameters {
		param := sw.Parameters[name]
		rewriteParameterRefs(&param, mapping)
		sw.Parameters[name] = param
	}
	for name := range sw.Responses {
		resp := sw.Responses[name]
		rewriteResponseRefs(&resp, mapping)
		sw.Responses[name] = resp
	}
}

func rewriteParameterRefs(param *spec.Parameter, mapping map[string]string) {
	if param == nil {
		return
	}
	if param.Schema != nil {
		rewriteSchemaRefs(param.Schema, mapping)
	}
}

func rewriteResponseRefs(resp *spec.Response, mapping map[string]string) {
	if resp == nil {
		return
	}
	if resp.Schema != nil {
		rewriteSchemaRefs(resp.Schema, mapping)
	}
}

func rewriteSchemaOrArrayRefs(soa *spec.SchemaOrArray, mapping map[string]string) {
	if soa == nil {
		return
	}
	if soa.Schema != nil {
		rewriteSchemaRefs(soa.Schema, mapping)
	}
	for i := range soa.Schemas {
		rewriteSchemaRefs(&soa.Schemas[i], mapping)
	}
}

func rewriteSchemaRefs(s *spec.Schema, mapping map[string]string) {
	if s == nil {
		return
	}
	rewriteRef(&s.Ref, mapping)
	rewriteSchemaOrArrayRefs(s.Items, mapping)
	if s.AdditionalItems != nil && s.AdditionalItems.Schema != nil {
		rewriteSchemaRefs(s.AdditionalItems.Schema, mapping)
	}
	for i := range s.AllOf {
		rewriteSchemaRefs(&s.AllOf[i], mapping)
	}
	for i := range s.AnyOf {
		rewriteSchemaRefs(&s.AnyOf[i], mapping)
	}
	for i := range s.OneOf {
		rewriteSchemaRefs(&s.OneOf[i], mapping)
	}
	if s.Not != nil {
		rewriteSchemaRefs(s.Not, mapping)
	}
	for name := range s.Properties {
		sub := s.Properties[name]
		rewriteSchemaRefs(&sub, mapping)
		s.Properties[name] = sub
	}
	for name := range s.PatternProperties {
		sub := s.PatternProperties[name]
		rewriteSchemaRefs(&sub, mapping)
		s.PatternProperties[name] = sub
	}
	if s.AdditionalProperties != nil {
		if s.AdditionalProperties.Schema != nil {
			rewriteSchemaRefs(s.AdditionalProperties.Schema, mapping)
		}
	}
}

// rewriteRef remaps a local "#/definitions/<name>" reference through mapping.
// Non-local references are left untouched.
func rewriteRef(ref *spec.Ref, mapping map[string]string) {
	if ref == nil || ref.String() == "" {
		return
	}
	u := ref.GetURL()
	if u.Scheme != "" || u.Host != "" || u.Path != "" {
		return
	}
	const prefix = "/definitions/"
	if !strings.HasPrefix(u.Fragment, prefix) {
		return
	}
	name := u.Fragment[len(prefix):]
	if newName, ok := mapping[name]; ok {
		*ref = spec.MustCreateRef("#/definitions/" + newName)
	}
}
