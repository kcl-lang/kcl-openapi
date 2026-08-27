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
	"path/filepath"
	"runtime"
	"testing"
)

// isOpenAPI3Header is the workhorse version check. detectOpenAPI3
// feeds it both the raw header (for JSON-form specs) and a comment-
// stripped version (for YAML), so the tests cover both paths.
func TestIsOpenAPI3Header(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		strip bool
		want  bool
	}{
		// YAML: feed stripped input, because the call site strips
		// comments and string contents before this check.
		{"yaml 3.0 with space", "openapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\n", true, true},
		{"yaml 3.1 with space", "openapi: 3.1.0\ninfo:\n  title: t\n  version: 0.1.0\n", true, true},
		{"yaml 3.0 no space", "openapi:3.0.0\ninfo:\n  title: t\n  version: 0.1.0\n", true, true},
		{"yaml 2.0", "swagger: '2.0'\ninfo:\n  title: t\n  version: 0.1.0\n", true, false},
		{"yaml with leading blanks", "\n\nopenapi: 3.0.0\ninfo:\n  title: t\n", true, true},

		// JSON: feed raw input, because the version is inside a
		// quoted string that stripHeaderNoise would blank out.
		{"json 3.0", `{"openapi":"3.0.0","info":{"title":"t","version":"0.1.0"}}`, false, true},
		{"json 3.0 with space", `{"openapi": "3.0.0", "info": {"title": "t", "version": "0.1.0"}}`, false, true},
		{"json 2.0", `{"swagger":"2.0","info":{"title":"t","version":"0.1.0"}}`, false, false},

		// Edge cases.
		{"empty", "", false, false},

		// Negative cases after stripping.
		{"openapi only in string", `description: "openapi: 3.0.0 inside a string"`, true, false},
		{"openapi only in comment", "# openapi: 3.0.0\nswagger: '2.0'\n", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			if tc.strip {
				in = stripHeaderNoise(in)
			}
			got := isOpenAPI3Header(in)
			if got != tc.want {
				t.Errorf("isOpenAPI3Header(%q) = %v, want %v", in, got, tc.want)
			}
		})
	}
}

func TestStripHeaderNoise(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			// The comment branch consumes characters up to the line
			// terminator; the outer loop's i++ skips the '\n' itself,
			// so only the surviving rest of the input is emitted.
			// The single-quoted `'2.0'` then gets its body blanked.
			name: "comment stripped",
			in:   "# openapi: 3.0.0\nswagger: '2.0'\n",
			want: "swagger: '   '\n",
		},
		{
			// All quoted strings get their contents blanked —
			// including JSON keys. That's intentional: detectOpenAPI3
			// checks the raw header for JSON-form patterns before
			// stripping, so blanking keys here doesn't lose
			// information that we still need.
			name: "double-quoted string body blanked",
			in:   `{"openapi":"3.0.0"}`,
			want: `{"       ":"     "}`,
		},
		{
			name: "single-quoted string body blanked",
			in:   "openapi: '3.0.0'\n",
			want: "openapi: '     '\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripHeaderNoise(tc.in)
			if got != tc.want {
				t.Errorf("stripHeaderNoise(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEnsurePathsField(t *testing.T) {
	// ensurePathsField is called after json.MarshalIndent, so input
	// in tests should be pretty-printed JSON to mirror the real flow.
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			// Insertion lands before the closing `}`, so the comma
			// sits on its own line after the previous block's
			// trailing newline. The output isn't pretty — there's a
			// stray `\n,` — but it is valid JSON, which is all
			// loads.Spec cares about.
			name: "no paths → injected before closing brace",
			in:   "{\n  \"info\": {\n    \"title\": \"t\"\n  }\n}",
			want: "{\n  \"info\": {\n    \"title\": \"t\"\n  }\n,\n  \"paths\": {}}",
		},
		{
			name: "paths present → unchanged",
			in:   "{\n  \"paths\": {}\n}",
			want: "{\n  \"paths\": {}\n}",
		},
		{
			name: "no closing brace → unchanged",
			in:   `{"swagger":"2.0"`,
			want: `{"swagger":"2.0"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ensurePathsField([]byte(tc.in))
			if string(got) != tc.want {
				t.Errorf("ensurePathsField(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Make sure isOpenAPI3Header still triggers when the version is buried
// below the first non-blank line — e.g. for documents that lead with a
// comment, which stripHeaderNoise removes entirely.
func TestIsOpenAPI3Header_BuriedVersion(t *testing.T) {
	in := "# header comment\nopenapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\n"
	if !isOpenAPI3Header(stripHeaderNoise(in)) {
		t.Errorf("isOpenAPI3Header should detect openapi version after a comment")
	}
}

// Round-trip an OpenAPI 3.x spec through loadOpenAPI3 and verify the
// resulting loads.Document reports swagger 2.0 and exposes our Pet
// definition. This is the closest thing we have to a black-box test of
// the conversion path without depending on the integration runner.
func TestLoadOpenAPI3_BasicRoundTrip(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../pkg/swagger/generator/openapi3_test.go
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	spec := filepath.Join(root, "pkg", "swagger", "generator", "testdata",
		"integration", "openapi3_basic", "openapi3_basic.golden.yaml")

	doc, err := loadOpenAPI3(spec)
	if err != nil {
		t.Fatalf("loadOpenAPI3: %v", err)
	}
	if got, want := doc.Version(), "2.0"; got != want {
		t.Errorf("doc.Version() = %q, want %q", got, want)
	}
	defs := doc.Spec().Definitions
	pet, ok := defs["Pet"]
	if !ok {
		t.Fatalf("Pet definition missing from lowered spec; have=%v", defs)
	}
	id, ok := pet.Properties["id"]
	if !ok {
		t.Fatalf("Pet.id property missing")
	}
	if len(id.Type) != 1 || id.Type[0] != "integer" {
		t.Errorf("Pet.id type = %v, want [integer]", id.Type)
	}
}