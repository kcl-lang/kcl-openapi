// Copyright 2024 The KCL Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0

// Package generator: cel.go provides CEL -> KCL translation for the
// `x-kubernetes-validations` CRD extension.
//
// The parser, lexer, and macro expansion are delegated to cel-go
// (https://github.com/google/cel-go). We only own the code generator:
// walk the protobuf AST produced by cel-go's ParseAndCheck and emit an
// equivalent KCL boolean expression.
package generator

import (
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	decls "github.com/google/cel-go/checker/decls"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// celToKCL translates a CEL boolean expression into a KCL boolean
// expression suitable for use inside a schema's `check:` block.
//
// Constructs that cannot be translated are emitted as a placeholder of
// the form `__UNSUPPORTED_CEL_CALL_<name>__` so the caller can detect
// them and skip emitting the rule (the placeholder would not compile in
// KCL). cel-go itself only knows the standard CEL functions and macros,
// so anything CRD-specific (e.g. `duration()`, `isURL()`) already raises
// a compile-time error before we ever see it.
//
// Supported CEL constructs:
//
//	literals (int, float, string, bool)
//	identifiers, field access, index, presence tests (`has(e.f)`)
//	comparisons and logical operators (`&&` -> and, `||` -> or, `!` -> not)
//	arithmetic
//	standard CEL methods on strings:
//	    x.matches(r)     -> _regex_match(str(x), r)
//	    x.startsWith(s)  -> str(x).starts_with(s)
//	    x.endsWith(s)    -> str(x).ends_with(s)
//	    size(x)          -> len(x)
//	higher-order macros:
//	    x.all(v, pred)       -> all v in x { pred }
//	    x.exists(v, pred)    -> any v in x { pred }
//	    x.exists_one(v, p)  -> sum([1 for v in x if p]) == 1
//	ternary cond ? a : b  -> a if cond else b
func celToKCL(expr string) (string, error) {
	// Declare `self` so CEL's parser accepts it. The actual type doesn't
	// matter for code emission: we only need the parser to be happy.
	env, err := cel.NewEnv(cel.Declarations(decls.NewVar("self", decls.Dyn)))
	if err != nil {
		return "", fmt.Errorf("cel: new env: %w", err)
	}
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return "", fmt.Errorf("cel: parse: %w", iss.Err())
	}
	parsed, err := cel.AstToParsedExpr(ast)
	if err != nil {
		return "", fmt.Errorf("cel: to parsed expr: %w", err)
	}
	if parsed.GetExpr() == nil {
		return "", fmt.Errorf("cel: empty expression")
	}
	return emitKCL(parsed.GetExpr())
}

// emitKCL walks one protobuf CEL Expr and returns its KCL rendering.
func emitKCL(e *exprpb.Expr) (string, error) {
	if e == nil {
		return "", nil
	}
	switch k := e.GetExprKind().(type) {
	case *exprpb.Expr_ConstExpr:
		return emitConst(k.ConstExpr)
	case *exprpb.Expr_IdentExpr:
		return k.IdentExpr.GetName(), nil
	case *exprpb.Expr_SelectExpr:
		return emitSelect(k.SelectExpr)
	case *exprpb.Expr_CallExpr:
		return emitCall(k.CallExpr)
	case *exprpb.Expr_ComprehensionExpr:
		return emitComprehension(k.ComprehensionExpr)
	case *exprpb.Expr_ListExpr:
		return emitList(k.ListExpr)
	case *exprpb.Expr_StructExpr:
		return emitStruct(k.StructExpr)
	}
	return unsupportedPlaceholder("ast-node", fmt.Sprintf("%T", e.GetExprKind()))
}

// emitSelect translates a CEL field-select or presence-test. KCL `check:`
// blocks reference schema attributes directly without `self`, so when the
// operand is the bare identifier `self` we strip the prefix. When the
// select is a CEL presence test (`has(self.f)`), we translate it to the
// KCL equivalent `f != None` so the rule stays semantically meaningful
// inside a schema check. Non-`self` operands are kept verbatim so that
// constructs we don't know how to translate still produce a placeholder
// the caller can detect.
func emitSelect(s *exprpb.Expr_Select) (string, error) {
	isSelf := isSelfIdent(s.GetOperand())
	if s.GetTestOnly() {
		if isSelf {
			return fmt.Sprintf("(%s != None)", s.GetField()), nil
		}
		// `has(m.f)` for non-`self` operands: KCL has no `has` and we
		// can't express "attribute of an arbitrary value" safely, so
		// mark the rule unsupported and let the caller drop it.
		return unsupportedPlaceholder("has", s.GetField())
	}
	if isSelf {
		return s.GetField(), nil
	}
	operand, err := emitKCL(s.GetOperand())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s", operand, s.GetField()), nil
}

// isSelfIdent reports whether `e` is the bare CEL identifier `self`.
func isSelfIdent(e *exprpb.Expr) bool {
	id, ok := e.GetExprKind().(*exprpb.Expr_IdentExpr)
	if !ok {
		return false
	}
	return id.IdentExpr.GetName() == "self"
}

func emitConst(l *exprpb.Constant) (string, error) {
	switch v := l.GetConstantKind().(type) {
	case *exprpb.Constant_BoolValue:
		if v.BoolValue {
			return "True", nil
		}
		return "False", nil
	case *exprpb.Constant_Int64Value:
		return fmt.Sprintf("%d", v.Int64Value), nil
	case *exprpb.Constant_Uint64Value:
		return fmt.Sprintf("%d", v.Uint64Value), nil
	case *exprpb.Constant_DoubleValue:
		return fmt.Sprintf("%v", v.DoubleValue), nil
	case *exprpb.Constant_StringValue:
		return fmt.Sprintf("%q", v.StringValue), nil
	case *exprpb.Constant_BytesValue:
		return unsupportedPlaceholder("bytes", string(v.BytesValue))
	case *exprpb.Constant_NullValue:
		return "None", nil
	}
	return unsupportedPlaceholder("literal", fmt.Sprintf("%T", l.GetConstantKind()))
}

// emitCall translates a CallExpr. CEL represents operators as calls with
// names like `_&&_` and methods as calls with a non-nil target.
func emitCall(c *exprpb.Expr_Call) (string, error) {
	fn := c.GetFunction()

	// Ternary `cond ? a : b` is represented as a CallExpr with the
	// function `_?_:_` and three arguments. KCL's `check:` block does
	// not accept `if/else` expressions (the parser treats `else` as a
	// bare name in that context), so we surface the rule as
	// unsupported and let the caller drop it.
	if fn == "_?_:_" {
		return unsupportedPlaceholder("ternary", "")
	}

	// Binary operators
	switch fn {
	case "_&&_":
		lhs, err := emitKCL(c.GetArgs()[0])
		if err != nil {
			return "", err
		}
		rhs, err := emitKCL(c.GetArgs()[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s and %s)", lhs, rhs), nil
	case "_||_":
		lhs, err := emitKCL(c.GetArgs()[0])
		if err != nil {
			return "", err
		}
		rhs, err := emitKCL(c.GetArgs()[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s or %s)", lhs, rhs), nil
	case "!_":
		arg, err := emitKCL(c.GetArgs()[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("not (%s)", arg), nil
	case "-_", "+_":
		// Unary minus / plus. cel-go represents these with a single
		// trailing underscore (no leading one), e.g. `-_` for negation.
		arg, err := emitKCL(c.GetArgs()[0])
		if err != nil {
			return "", err
		}
		if fn == "+_" {
			return fmt.Sprintf("(+%s)", arg), nil
		}
		return fmt.Sprintf("(-%s)", arg), nil
	}

	// Binary arithmetic / comparison operators all have the form `_X_`
	// where X is the operator (==, !=, <, <=, >, >=, +, -, *, /, %).
	if strings.HasPrefix(fn, "_") && strings.HasSuffix(fn, "_") && len(fn) >= 3 {
		op := fn[1 : len(fn)-1]
		// `_index_` (`_[_]`) is handled below.
		if op != "" && op[0] != '[' {
			lhs, err := emitKCL(c.GetArgs()[0])
			if err != nil {
				return "", err
			}
			rhs, err := emitKCL(c.GetArgs()[1])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("(%s %s %s)", lhs, op, rhs), nil
		}
	}

	// Indexing `a[i]` -> `a[i]`. cel-go represents indexing as a
	// `_[_]` call with the receiver in args[0] and target=nil.
	if fn == "_[_]" {
		if len(c.GetArgs()) < 2 {
			return "", fmt.Errorf("cel: _[_] expects 2 args, got %d", len(c.GetArgs()))
		}
		target, err := emitKCL(c.GetArgs()[0])
		if err != nil {
			return "", err
		}
		idx, err := emitKCL(c.GetArgs()[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s[%s]", target, idx), nil
	}

	// `size(x)` is a global function in CEL.
	if fn == "size" {
		if len(c.GetArgs()) != 1 {
			return "", fmt.Errorf("cel: size() expects 1 arg, got %d", len(c.GetArgs()))
		}
		// `size(self)` asks for the field-count of the schema instance.
		// KCL `check:` blocks have no handle for the instance, so we drop
		// the rule rather than emit `len(self)` (undefined identifier).
		if isSelfIdent(c.GetArgs()[0]) {
			return unsupportedPlaceholder("size", "self")
		}
		arg, err := emitKCL(c.GetArgs()[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("len(%s)", arg), nil
	}

	// Method calls on a target. CEL's standard library has a few string
	// methods we map to KCL equivalents; everything else is reported as
	// unsupported so the caller can drop the rule.
	if c.GetTarget() != nil {
		target, err := emitKCL(c.GetTarget())
		if err != nil {
			return "", err
		}
		switch fn {
		case "matches":
			if len(c.GetArgs()) != 1 {
				return "", fmt.Errorf("cel: matches() expects 1 arg, got %d", len(c.GetArgs()))
			}
			arg, err := emitKCL(c.GetArgs()[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("_regex_match(str(%s), %s)", target, arg), nil
		case "startsWith":
			if len(c.GetArgs()) != 1 {
				return "", fmt.Errorf("cel: startsWith() expects 1 arg, got %d", len(c.GetArgs()))
			}
			arg, err := emitKCL(c.GetArgs()[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("str(%s).starts_with(%s)", target, arg), nil
		case "endsWith":
			if len(c.GetArgs()) != 1 {
				return "", fmt.Errorf("cel: endsWith() expects 1 arg, got %d", len(c.GetArgs()))
			}
			arg, err := emitKCL(c.GetArgs()[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("str(%s).ends_with(%s)", target, arg), nil
		}
		return unsupportedPlaceholder(fn, target)
	}

	return unsupportedPlaceholder(fn, "")
}

// emitComprehension handles the macros `all`, `exists`, and
// `exists_one`, which cel-go has already desugared into a fold over the
// iterator. We detect the macro by inspecting the initial accumulator
// value: bool true = `all`, bool false = `exists`, int 0 = `exists_one`.
func emitComprehension(c *exprpb.Expr_Comprehension) (string, error) {
	iterVar := c.GetIterVar()
	iterRange, err := emitKCL(c.GetIterRange())
	if err != nil {
		return "", err
	}
	accuVar := c.GetAccuVar()

	// Recover the user's predicate from the loop_step. cel-go expands
	// the macros into a fold with a fixed accumulator name
	// (`parser.AccumulatorName` == "__result__"); the user's predicate
	// sits in the second argument of the `_&&_`/`_||_` for all/exists,
	// or as the condition of the `_?_:_` for exists_one.
	predicate, err := extractPredicate(c.GetLoopStep(), accuVar)
	if err != nil {
		return "", err
	}

	switch init := c.GetAccuInit().GetExprKind().(type) {
	case *exprpb.Expr_ConstExpr:
		switch v := init.ConstExpr.GetConstantKind().(type) {
		case *exprpb.Constant_BoolValue:
			if v.BoolValue {
				// all: accuInit=true, step=`__result__ && P`
				return fmt.Sprintf("all %s in %s {\n    %s\n}", iterVar, iterRange, predicate), nil
			}
			// exists: accuInit=false, step=`__result__ || P`
			return fmt.Sprintf("any %s in %s {\n    %s\n}", iterVar, iterRange, predicate), nil
		case *exprpb.Constant_Int64Value:
			if v.Int64Value == 0 {
				// exists_one: accuInit=0, step=`P ? __result__+1 : __result__`
				return fmt.Sprintf("sum([1 for %s in %s if %s]) == 1", iterVar, iterRange, predicate), nil
			}
		}
	}
	return unsupportedPlaceholder("comprehension", fmt.Sprintf("accu=%s", accuVar))
}

// extractPredicate walks a comprehension's loop_step and returns the
// user's predicate expression. cel-go desugars the standard macros
// into one of three shapes:
//
//	all:        accu && P   (CallExpr _&&_, args[0]=accu, args[1]=P)
//	exists:     accu || P   (CallExpr _||_, args[0]=accu, args[1]=P)
//	exists_one: P ? accu+1 : accu   (CallExpr _?_:_, args[0]=P, args[1]=accu+1, args[2]=accu)
//
// accuVar is always "__result__" per cel-go's parser/macro.go.
func extractPredicate(step *exprpb.Expr, accuVar string) (string, error) {
	call, ok := step.GetExprKind().(*exprpb.Expr_CallExpr)
	if !ok {
		return "", fmt.Errorf("cel: comprehension step is not a call: %T", step.GetExprKind())
	}
	switch call.CallExpr.GetFunction() {
	case "_&&_", "_||_":
		a := call.CallExpr.GetArgs()[0]
		b := call.CallExpr.GetArgs()[1]
		// cel-go always puts the accumulator first, but accept either order
		// in case a future version rearranges it.
		if isIdent(a, accuVar) {
			return emitKCL(b)
		}
		if isIdent(b, accuVar) {
			return emitKCL(a)
		}
		return "", fmt.Errorf("cel: comprehension step missing accumulator %q", accuVar)
	case "_?_:_":
		// For exists_one the user's predicate is the ternary's condition.
		return emitKCL(call.CallExpr.GetArgs()[0])
	}
	return "", fmt.Errorf("cel: unexpected comprehension step function %q", call.CallExpr.GetFunction())
}

// isIdent reports whether `e` is a bare identifier expression with the
// given name. Used to detect the accumulator inside a comprehension step.
func isIdent(e *exprpb.Expr, name string) bool {
	id, ok := e.GetExprKind().(*exprpb.Expr_IdentExpr)
	if !ok {
		return false
	}
	return id.IdentExpr.GetName() == name
}

func emitList(l *exprpb.Expr_CreateList) (string, error) {
	parts := make([]string, 0, len(l.GetElements()))
	for _, e := range l.GetElements() {
		s, err := emitKCL(e)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

func emitStruct(s *exprpb.Expr_CreateStruct) (string, error) {
	// Map/object literal -> KCL dict literal.
	parts := make([]string, 0, len(s.GetEntries()))
	for _, entry := range s.GetEntries() {
		key, err := emitKCL(entry.GetMapKey())
		if err != nil {
			return "", err
		}
		val, err := emitKCL(entry.GetValue())
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%s: %s", key, val))
	}
	return "{" + strings.Join(parts, ", ") + "}", nil
}

// unsupportedPlaceholder returns a marker that callers use to detect
// CEL constructs the translator cannot represent in KCL. The marker is
// deliberately non-compiling so a downstream consumer can either drop
// the rule (k8s_validations.go) or surface it loudly.
func unsupportedPlaceholder(fn string, ctx string) (string, error) {
	if ctx == "" {
		return fmt.Sprintf("__UNSUPPORTED_CEL_CALL_%s__", fn), nil
	}
	return fmt.Sprintf("__UNSUPPORTED_CEL_CALL_%s(%s)__", fn, ctx), nil
}

// isUnsupportedKCL returns true when `s` contains a marker emitted by
// unsupportedPlaceholder. Used by k8s_validations.go to drop rules we
// could not translate.
func isUnsupportedKCL(s string) bool {
	return strings.Contains(s, "__UNSUPPORTED_CEL_CALL_")
}
