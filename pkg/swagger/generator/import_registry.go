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
	"log"
	"strings"
)

// ImportAliasRegistry assigns import aliases to the external packages used
// by a generation run. KCL binds imports in the package scope: every file of
// a generated package that imports the same external package must use the
// same alias, and two different packages must not claim the same alias. The
// per-file logic in collectImports alone cannot guarantee that (e.g. one
// file importing k8s.api.core.v1 and another k8s.apimachinery.pkg.apis.meta.v1
// would both bind `v1`), so aliases are assigned once per generation, in a
// deterministic order, and reused across files.
type ImportAliasRegistry struct {
	byPkg map[string]string
	used  map[string]bool
}

func NewImportAliasRegistry() *ImportAliasRegistry {
	return &ImportAliasRegistry{
		byPkg: make(map[string]string),
		used:  make(map[string]bool),
	}
}

// claim returns the alias for pkg, assigning a new unique one on first use.
// fileImps holds the imports already collected for the current file so the
// alias also avoids collisions within the file itself.
func (r *ImportAliasRegistry) claim(pkg, innerPkg, module string, fileImps map[string]importStmt) string {
	if as, ok := r.byPkg[pkg]; ok {
		return as
	}
	parts := strings.Split(innerPkg, ".")
	asName := ""
	taken := false
	for i := len(parts) - 1; i >= 0; i-- {
		// walk the package path from the leaf upwards, prefixing the parent
		// segment on every conflict, e.g. v1 -> coreV1 -> apiCoreV1
		asName = parts[i] + strings.ToTitle(asName)
		taken = r.used[asName]
		if !taken {
			for _, v := range fileImps {
				if v.AsName == asName {
					taken = true
					break
				}
			}
		}
		if !taken {
			break
		}
	}
	if taken {
		mangled := "kclMangled" + strings.ToTitle(asName)
		log.Printf("[WARN] the import paths in module %s.%s are confict, please resolve it properly", pkg, module)
		asName = mangled
	}
	r.byPkg[pkg] = asName
	r.used[asName] = true
	return asName
}
