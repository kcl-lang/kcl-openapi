// Copyright 2024 The KCL Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0

// Package generator: openapi3.go adds OpenAPI 3.0/3.1 support on top
// of the Swagger 2.0-only go-openapi/loads machinery.
//
// We use kin-openapi (https://github.com/getkin/kin-openapi) to parse
// the 3.x spec, then kin-openapi's openapi2conv.FromV3 to lower it
// to a Swagger 2.0 *openapi2.T. The lowered spec is then handed to
// the rest of the pipeline via a temp file, so the existing loads.Spec
// path (validation, flattening, kcl-emission) doesn't have to know
// the input was ever a 3.x spec.
package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/go-openapi/loads"
)

// loadOpenAPI3 reads an OpenAPI 3.x spec from `path`, converts it to
// Swagger 2.0, and returns a *loads.Document that the rest of the
// pipeline can consume.
//
// Returns an error if the spec can't be parsed as OpenAPI 3.x or if
// the lowering to 2.0 fails. If the input is Swagger 2.0 the caller
// should fall through to loads.Spec instead.
func loadOpenAPI3(path string) (*loads.Document, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc3, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("openapi3: load %q: %w", path, err)
	}
	if err := doc3.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("openapi3: validate %q: %w", path, err)
	}

	doc2, err := openapi2conv.FromV3(doc3)
	if err != nil {
		return nil, fmt.Errorf("openapi3: convert to swagger 2.0: %w", err)
	}
	// Swagger 2.0 requires the `paths` field even when it's empty;
	// kin-openapi's converter omits it for an empty OpenAPI 3.x
	// paths map. writeOpenAPI2Temp re-injects the field after
	// marshaling because the openapi2.Swagger struct tags it
	// `omitempty`.
	if doc2.Paths == nil {
		doc2.Paths = map[string]*openapi2.PathItem{}
	}

	// We can't pass a *loads.Document directly across the existing
	// pipeline boundaries because loads.Spec is what sets the
	// specFilePath used for $ref resolution later. Write the lowered
	// doc to a temp file and let loads.Spec pick it up.
	tmp, err := writeOpenAPI2Temp(doc2, filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("openapi3: write temp swagger 2.0: %w", err)
	}
	defer os.Remove(tmp)

	specDoc, err := loads.Spec(tmp)
	if err != nil {
		return nil, fmt.Errorf("openapi3: load swagger 2.0: %w", err)
	}
	return specDoc, nil
}

// writeOpenAPI2Temp marshals `doc2` to a temp JSON file alongside the
// input spec and returns its path. We use JSON (not YAML) because
// loads.Spec uses go-openapi/swag for YAML→JSON conversion which
// expects the yaml.v2 MapSlice-based representation; serializing to
// JSON sidesteps that whole path.
//
// `baseName` is the input file's basename; the temp file lives in the
// same directory so any relative $refs (rare in 3.x lowered to 2.x
// but possible if the converter leaves them intact) resolve the way
// they would in the source tree.
//
// Swagger 2.0 requires the `paths` field even when it's empty, but
// kin-openapi's openapi2.Swagger struct tags it with `omitempty` so
// an empty map gets stripped from the JSON. We re-inject the field
// after marshaling so loads.Spec + the validator are happy.
func writeOpenAPI2Temp(doc2 interface{}, baseName string) (string, error) {
	raw, err := json.MarshalIndent(doc2, "", "  ")
	if err != nil {
		return "", err
	}
	// Inject "paths": {} right after "swagger" if it isn't present.
	raw = ensurePathsField(raw)

	dir := filepath.Dir(baseName)
	if dir == "" || dir == "." {
		// baseName was just a name; put the temp in the cwd.
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, "kcl-openapi3-*.json")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// ensurePathsField guarantees that the marshaled JSON contains a
// `"paths":{}` entry. If paths is already present (non-empty),
// returns the input unchanged. We avoid a full unmarshal-remarshal
// round-trip so order and other quirks of the kin-openapi output are
// preserved.
func ensurePathsField(raw []byte) []byte {
	if bytes.Contains(raw, []byte(`"paths"`)) {
		return raw
	}
	// Find the closing brace of the swagger object — we want to
	// insert paths before it. The output is pretty-printed so the
	// pattern is reliable; otherwise we'd unmarshal.
	const insert = ",\n  \"paths\": {}"
	end := bytes.LastIndexByte(raw, '}')
	if end < 0 {
		return raw
	}
	out := make([]byte, 0, len(raw)+len(insert))
	out = append(out, raw[:end]...)
	out = append(out, insert...)
	out = append(out, raw[end:]...)
	return out
}

// detectOpenAPI3 peeks at the first few hundred bytes of `path` and
// reports whether the document declares itself as OpenAPI 3.x. We
// accept both YAML ("openapi: 3.x") and JSON ("openapi":"3.x").
// Detection is best-effort: a malformed header doesn't error, it
// just returns false so the caller falls through to the Swagger 2.0
// loader which will produce a clearer error.
func detectOpenAPI3(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// 4 KiB is enough for any reasonable header; the openapi version
	// field is always within the first few hundred bytes.
	const peekSize = 4096
	buf := make([]byte, peekSize)
	n, _ := io.ReadFull(f, buf)
	header := string(buf[:n])

	// Strip line comments and string contents so an "openapi"
	// mention inside a comment or example doesn't trick us. We check
	// the raw header first because JSON-form specs declare their
	// version inside a quoted string, which stripHeaderNoise would
	// blank out.
	if isOpenAPI3Header(header) {
		return true, nil
	}
	stripped := stripHeaderNoise(header)
	return isOpenAPI3Header(stripped), nil
}

// isOpenAPI3Header reports whether `header` declares itself as
// OpenAPI 3.x. Both YAML ("openapi: 3.x") and JSON ("openapi":"3.x")
// forms are accepted; whitespace after the colon is optional in YAML.
//
// `header` may be the raw file header or a comment-and-string-stripped
// version. detectOpenAPI3 feeds it both: the raw form for JSON
// (whose version field is itself inside a quoted string that
// stripHeaderNoise would blank out) and the stripped form for YAML
// (so an "openapi" mention in a comment or example doesn't match).
func isOpenAPI3Header(header string) bool {
	prefixes := []string{
		`openapi: `,
		`openapi:`,
		`"openapi":"3`,
		`"openapi": "3`,
	}
	for _, p := range prefixes {
		if strings.HasPrefix(header, p) {
			return true
		}
	}
	// Also match anywhere near the top in case there are leading
	// blank lines or an unusual encoding pragma.
	for _, p := range []string{
		"\nopenapi: ",
		"\nopenapi:",
		`"openapi":"3`,
		`"openapi": "3`,
	} {
		if strings.Contains(header[:trimLen(header, 4096)], p) {
			return true
		}
	}
	return false
}

// trimLen returns min(len(s), n).
func trimLen(s string, n int) int {
	if len(s) < n {
		return len(s)
	}
	return n
}

// stripHeaderNoise removes "#..." comments and the contents of all
// quoted strings so version detection only matches the top-level
// version field. It's intentionally crude: any false negative just
// means we fall through to the 2.0 loader.
func stripHeaderNoise(s string) string {
	var out bytes.Buffer
	out.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '#':
			// line comment; skip to newline
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case '"', '\'':
			// quoted string; copy the quotes but blank out the body
			out.WriteByte(c)
			i++
			for i < len(s) && s[i] != c {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				out.WriteByte(' ')
				i++
			}
			if i < len(s) {
				out.WriteByte(s[i]) // closing quote
			}
		default:
			out.WriteByte(c)
		}
	}
	return out.String()
}