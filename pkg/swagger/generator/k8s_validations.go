// Copyright 2024 The KCL Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0

package generator

import (
	"log"
	"strings"

	"github.com/go-openapi/spec"
)

// extractK8sValidations inspects the schema for an `x-kubernetes-validations`
// extension and translates each CEL rule to a KCL boolean expression.
//
// Behaviour:
//   - A nil/empty `x-kubernetes-validations` returns nil.
//   - Each entry must be an object with a non-empty `rule` string; entries
//     missing the field are skipped (logged).
//   - Translation failures and rules that depend on unsupported CEL
//     functions (the placeholder `__UNSUPPORTED_CEL_CALL_*` marker) are
//     skipped with a logged warning rather than emitted into the
//     generated KCL. This keeps generated files compilable while still
//     surfacing the failed rules to operators via the log.
//
// The function is deliberately conservative: it never returns an error and
// never panics. Operators should monitor the generation log for skipped
// rules that may need manual handling.
func extractK8sValidations(schema spec.Schema) []CheckExpr {
	raw, ok := schema.Extensions[xK8sValidations]
	if !ok || raw == nil {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		log.Printf("[WARN] x-kubernetes-validations: expected array, got %T", raw)
		return nil
	}
	out := make([]CheckExpr, 0, len(list))
	for i, item := range list {
		entry, ok := item.(map[string]interface{})
		if !ok {
			log.Printf("[WARN] x-kubernetes-validations[%d]: expected object, got %T", i, item)
			continue
		}
		ruleRaw, ok := entry["rule"].(string)
		if !ok || strings.TrimSpace(ruleRaw) == "" {
			log.Printf("[WARN] x-kubernetes-validations[%d]: missing 'rule' string", i)
			continue
		}
		msgRaw, _ := entry["message"].(string)
		expr, err := celToKCL(ruleRaw)
		if err != nil {
			log.Printf("[WARN] x-kubernetes-validations[%d]: CEL translation failed: %v (rule: %s)", i, err, ruleRaw)
			continue
		}
		if isUnsupportedKCL(expr) {
			log.Printf("[WARN] x-kubernetes-validations[%d]: skipping rule that uses unsupported CEL construct: %s", i, ruleRaw)
			continue
		}
		out = append(out, CheckExpr{
			Expr:    expr,
			Rule:    ruleRaw,
			Message: msgRaw,
		})
	}
	return out
}