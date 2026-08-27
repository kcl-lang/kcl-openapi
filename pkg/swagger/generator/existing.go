// Copyright 2024 The KCL Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0

package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// ExistingModel describes a directory of pre-existing KCL models that the
// generator should reference via `import` instead of regenerating.
//
// When the generator encounters a schema in the spec whose name matches a
// schema discovered in an ExistingModel.Path, it skips the generation of
// that schema's file and instead emits cross-package references as
// `<alias>.<SchemaName>` (backed by an `import <alias>` statement).
type ExistingModel struct {
	// Alias is the import alias referenced by the generator when emitting
	// cross-package references to schemas discovered in Path.
	Alias string
	// Path is the directory containing pre-existing KCL model files (*.k).
	Path string
}

// schemaNameRegexp matches KCL `schema <Name>` (optionally followed by
// `(Base1, Base2)` for protocol/mixin inheritance) followed by a colon.
// It deliberately ignores indented schema bodies (e.g. items declared
// inside a list or dict) by requiring the leading whitespace to be zero or
// short.
var schemaNameRegexp = regexp.MustCompile(`(?m)^[ \t]{0,8}schema[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*(?:\([^)]*\))?[ \t]*:`)

// LoadExistingModels scans each existing-model directory for `.k` files
// and extracts schema names. It returns a map keyed by the discovered
// schema name to its alias, suitable for use as GenOpts.ExistingDefs.
//
// The function performs the following sanity checks:
//   - each existing-model entry has a non-empty alias and path
//   - aliases are unique across entries
//   - the same schema name does not appear under two different aliases
//
// On error, the partial result is discarded.
func LoadExistingModels(existing []ExistingModel) (map[string]string, error) {
	if len(existing) == 0 {
		return nil, nil
	}
	// out keeps only the alias because that's what GenOpts.ExistingDefs
	// expects. We retain the path of the *first* declaration of each
	// schema name in schemaSources so a conflict can report the directory
	// the duplicate was originally discovered in.
	out := make(map[string]string)
	aliasPaths := make(map[string]string)
	schemaSources := make(map[string]string)
	for _, e := range existing {
		if e.Alias == "" || e.Path == "" {
			return nil, fmt.Errorf("existing-model entry requires non-empty alias and path, got %+v", e)
		}
		if prev, ok := aliasPaths[e.Alias]; ok && prev != e.Path {
			return nil, fmt.Errorf("existing-model alias %q is reused by %q and %q", e.Alias, prev, e.Path)
		}
		aliasPaths[e.Alias] = e.Path
	}
	for _, e := range existing {
		files, err := filepath.Glob(filepath.Join(e.Path, "*.k"))
		if err != nil {
			return nil, fmt.Errorf("scan existing-models dir %q: %v", e.Path, err)
		}
		sort.Strings(files)
		for _, f := range files {
			content, err := os.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("read existing-model file %q: %v", f, err)
			}
			for _, m := range schemaNameRegexp.FindAllStringSubmatch(string(content), -1) {
				name := m[1]
				if prev, ok := out[name]; ok && prev != e.Alias {
					return nil, fmt.Errorf("schema %q is declared in multiple existing-model directories (%q under alias %q and %q under alias %q)",
						name, schemaSources[name], prev, e.Path, e.Alias)
				}
				if _, already := out[name]; !already {
					schemaSources[name] = e.Path
				}
				out[name] = e.Alias
			}
		}
	}
	return out, nil
}
