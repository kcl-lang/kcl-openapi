// Copyright 2024 The KCL Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0

package generator

import (
	"strings"
	"testing"
)

func TestCelToKCL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"literal int", "42", "42"},
		{"literal float", "3.14", "3.14"},
		{"literal bool true", "true", "True"},
		{"literal bool false", "false", "False"},
		{"literal dq string", `"abc"`, `"abc"`},
		{"literal sq string", `'abc'`, `"abc"`},

		{"self ref", "self", "self"},
		{"field access", "self.name", "name"},
		{"deep field", "self.a.b.c", "a.b.c"},
		{"index expr", "self.list[0]", "list[0]"},

		{"eq", `self.name == "x"`, `(name == "x")`},
		{"neq", `self.x != "y"`, `(x != "y")`},
		{"lt", "self.a < 10", "(a < 10)"},
		{"lte", "self.a <= 10", "(a <= 10)"},
		{"gt", "self.a > 10", "(a > 10)"},
		{"gte", "self.a >= 10", "(a >= 10)"},

		{"and", `self.a == 1 && self.b == 2`, `((a == 1) and (b == 2))`},
		{"or", `self.a == 1 || self.b == 2`, `((a == 1) or (b == 2))`},
		{"not", `!self.a`, `not (a)`},
		{"not has", `!has(self.foo)`, `not ((foo != None))`},
		{"precedence", `self.a || self.b && self.c`, `(a or (b and c))`},
		{"paren group", `(self.a || self.b) && self.c`, `((a or b) and c)`},

		{"arith add", "self.a + self.b", "(a + b)"},
		{"arith mul precedence", "self.a + self.b * self.c", "(a + (b * c))"},
		{"unary minus", "-self.a", "(-a)"},

		// KCL's `check:` block does not accept `if/else` expressions (the
		// parser treats `else` as a bare name there), so the translator
		// surfaces ternary as an unsupported construct and the caller
		// drops the rule.
		{"ternary", `self.a ? "x" : "y"`, `__UNSUPPORTED_CEL_CALL_ternary__`},

		{"size", "size(self.list)", "len(list)"},
		{"size self", "size(self)", "__UNSUPPORTED_CEL_CALL_size(self)__"},
		{"matches", `self.email.matches("^.+@.+$")`, `_regex_match(str(email), "^.+@.+$")`},
		{"startsWith", `self.name.startsWith("foo")`, `str(name).starts_with("foo")`},
		{"endsWith", `self.name.endsWith("bar")`, `str(name).ends_with("bar")`},
		{"has", "has(self.foo)", "(foo != None)"},

		{"all", `self.list.all(x, x > 0)`, "all x in list {\n    (x > 0)\n}"},
		{"exists", `self.list.exists(x, x == "ok")`, "any x in list {\n    (x == \"ok\")\n}"},
		{"exists_one", `self.list.exists_one(x, x == 1)`,
			"sum([1 for x in list if (x == 1)]) == 1"},

		{"complex real", `self.minReplicas <= self.maxReplicas && size(self.list) > 0`,
			`((minReplicas <= maxReplicas) and (len(list) > 0))`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := celToKCL(c.in)
			if err != nil {
				t.Fatalf("celToKCL(%q): unexpected error: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("celToKCL(%q):\n got: %s\nwant: %s", c.in, got, c.want)
			}
			// The `want` value is itself a `__UNSUPPORTED_CEL_CALL_*`
			// marker for cases the translator deliberately cannot
			// render (e.g. ternary inside a KCL `check:` block). Those
			// rules are dropped by k8s_validations.go.
			wantUnsupported := strings.HasPrefix(c.want, "__UNSUPPORTED_CEL_CALL_")
			if isUnsupportedKCL(got) && !wantUnsupported {
				t.Errorf("celToKCL(%q) produced unsupported call placeholder: %s", c.in, got)
			}
		})
	}
}

func TestCelToKCL_Unsupported(t *testing.T) {
	// Use a CEL standard-library method that we deliberately don't
	// translate. cel-go type-checks the expression, so an unknown
	// identifier (unicornsFly) is rejected at compile time and never
	// reaches our walker; instead we use `contains`, a real CEL string
	// method we don't yet map to a KCL equivalent.
	got, err := celToKCL(`self.name.contains("x")`)
	if err != nil {
		t.Fatalf("expected no parse error, got %v", err)
	}
	if !strings.Contains(got, "__UNSUPPORTED_CEL_CALL_contains") {
		t.Errorf("expected unsupported marker, got %s", got)
	}
}

func TestCelToKCL_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"unterminated string", `self.x == "abc`},
		{"stray colon", `self.x :`},
		{"unknown identifier", `self.foo.unicornsFly()`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := celToKCL(c.in); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}
