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
		{"field access", "self.name", "self.name"},
		{"deep field", "self.a.b.c", "self.a.b.c"},
		{"index expr", "self.list[0]", "self.list[0]"},

		{"eq", `self.name == "x"`, `(self.name == "x")`},
		{"neq", `self.x != "y"`, `(self.x != "y")`},
		{"lt", "self.a < 10", "(self.a < 10)"},
		{"lte", "self.a <= 10", "(self.a <= 10)"},
		{"gt", "self.a > 10", "(self.a > 10)"},
		{"gte", "self.a >= 10", "(self.a >= 10)"},

		{"and", `self.a == 1 && self.b == 2`, `((self.a == 1) and (self.b == 2))`},
		{"or", `self.a == 1 || self.b == 2`, `((self.a == 1) or (self.b == 2))`},
		{"not", `!self.a`, `not (self.a)`},
		{"precedence", `self.a || self.b && self.c`, `(self.a or (self.b and self.c))`},
		{"paren group", `(self.a || self.b) && self.c`, `((self.a or self.b) and self.c)`},

		{"arith add", "self.a + self.b", "(self.a + self.b)"},
		{"arith mul precedence", "self.a + self.b * self.c", "(self.a + (self.b * self.c))"},
		{"unary minus", "-self.a", "(-self.a)"},

		{"ternary", `self.a ? "x" : "y"`, `("x") if (self.a) else ("y")`},

		{"size", "size(self.list)", "len(self.list)"},
		{"matches", `self.email.matches("^.+@.+$")`, `_regex_match(str(self.email), "^.+@.+$")`},
		{"startsWith", `self.name.startsWith("foo")`, `str(self.name).starts_with("foo")`},
		{"endsWith", `self.name.endsWith("bar")`, `str(self.name).ends_with("bar")`},
		{"has", "has(self.foo)", "(has self.foo)"},

		{"all", `self.list.all(x, x > 0)`, "all x in self.list {\n    (x > 0)\n}"},
		{"exists", `self.list.exists(x, x == "ok")`, "any x in self.list {\n    (x == \"ok\")\n}"},
		{"exists_one", `self.list.exists_one(x, x == 1)`,
			"sum([1 for x in self.list if (x == 1)]) == 1"},

		{"complex real", `self.minReplicas <= self.maxReplicas && size(self.list) > 0`,
			`((self.minReplicas <= self.maxReplicas) and (len(self.list) > 0))`},
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
			if isUnsupportedKCL(got) {
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