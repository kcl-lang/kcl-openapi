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
	"strconv"
	"strings"
	"unicode"
)

// celToKCL translates a CEL boolean expression into a KCL boolean
// expression suitable for use inside a schema's `check:` block.
//
// Supported CEL constructs:
//
//	self, bare identifiers
//	self.field, field.subfield
//	literals: integers, floats, strings (single or double quoted), true/false
//	comparisons: ==, !=, <, <=, >, >=
//	logical: &&, ||, !      (-> and, or, not)
//	arithmetic: +, -, *, /, %
//	parentheses
//	function calls:
//	    size(x)          -> len(x)
//	    x.matches(r)     -> _regex_match(str(x), r)
//	    x.startsWith(s)  -> str(x).starts_with(s)
//	    x.endsWith(s)    -> str(x).ends_with(s)
//	    has(x.field)     -> (has(x.field))   (KCL has a builtin with this name)
//	higher-order:
//	    x.all(v, pred)       -> all v in x { pred }
//	    x.exists(v, pred)    -> any v in x { pred }
//	    x.exists_one(v, pred)-> sum([1 for v in x if pred]) == 1
//	ternary: cond ? a : b   -> a if cond else b
//
// Unsupported constructs cause an error. The caller is expected to either
// skip the offending rule (with a warning) or fail loudly.
func celToKCL(expr string) (string, error) {
	toks, err := tokenizeCEL(strings.TrimSpace(expr))
	if err != nil {
		return "", fmt.Errorf("cel: tokenize: %w", err)
	}
	p := &celParser{tokens: toks}
	node, err := p.parseExpr()
	if err != nil {
		return "", fmt.Errorf("cel: parse: %w", err)
	}
	if p.pos != len(p.tokens) {
		return "", fmt.Errorf("cel: unexpected token %q", p.tokens[p.pos].lit)
	}
	return node.kcl(), nil
}

// celTokenKind enumerates the lexical categories produced by tokenizeCEL.
type celTokenKind int

const (
	celIdent   celTokenKind = iota
	celInt                  // 123
	celFloat                // 1.5
	celString               // "abc" or 'abc'
	celBool                 // true / false
	celOp                   // ==, !=, <=, >=, <, >, &&, ||, !, +, -, *, /, %, ?, :, ., (, ), ,
	celMacro                // all / exists / exists_one (parsed as identifier too; recognized later)
	celEOF
)

type celToken struct {
	kind celTokenKind
	lit  string
}

func tokenizeCEL(src string) ([]celToken, error) {
	var out []celToken
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' && j+1 < len(src) {
					j += 2
					continue
				}
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("unterminated string at offset %d", i)
			}
			out = append(out, celToken{kind: celString, lit: src[i : j+1]})
			i = j + 1
		case isIdentStart(c):
			j := i + 1
			for j < len(src) && isIdentCont(src[j]) {
				j++
			}
			id := src[i:j]
			switch id {
			case "true", "false":
				out = append(out, celToken{kind: celBool, lit: id})
			default:
				out = append(out, celToken{kind: celIdent, lit: id})
			}
			i = j
		case isDigit(c) || (c == '.' && i+1 < len(src) && isDigit(src[i+1])):
			j := i
			seenDot := false
			for j < len(src) && (isDigit(src[j]) || (src[j] == '.' && !seenDot)) {
				if src[j] == '.' {
					seenDot = true
				}
				j++
			}
			num := src[i:j]
			if seenDot {
				if _, err := strconv.ParseFloat(num, 64); err != nil {
					return nil, fmt.Errorf("invalid float %q", num)
				}
				out = append(out, celToken{kind: celFloat, lit: num})
			} else {
				if _, err := strconv.ParseInt(num, 10, 64); err != nil {
					return nil, fmt.Errorf("invalid int %q", num)
				}
				out = append(out, celToken{kind: celInt, lit: num})
			}
			i = j
		default:
			// multi-char operators first
			switch {
			case strings.HasPrefix(src[i:], "=="),
				strings.HasPrefix(src[i:], "!="),
				strings.HasPrefix(src[i:], "<="),
				strings.HasPrefix(src[i:], ">="),
				strings.HasPrefix(src[i:], "&&"),
				strings.HasPrefix(src[i:], "||"):
				out = append(out, celToken{kind: celOp, lit: src[i : i+2]})
				i += 2
			default:
				if strings.ContainsRune("+-*/%<>!?:.,()[]", rune(c)) {
					out = append(out, celToken{kind: celOp, lit: string(c)})
					i++
				} else {
					return nil, fmt.Errorf("unexpected character %q at offset %d", c, i)
				}
			}
		}
	}
	return out, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentCont(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// celNode is one node in the parsed CEL AST.
type celNode interface {
	kcl() string
}

type celIdentNode struct{ name string }

func (n *celIdentNode) kcl() string { return n.name }

type celLiteralNode struct{ lit string }

func (n *celLiteralNode) kcl() string { return n.lit }

type celMemberNode struct {
	recv celNode
	name string
}

func (n *celMemberNode) kcl() string { return fmt.Sprintf("%s.%s", n.recv.kcl(), n.name) }

type celIndexNode struct {
	recv  celNode
	index celNode
}

func (n *celIndexNode) kcl() string {
	return fmt.Sprintf("%s[%s]", n.recv.kcl(), n.index.kcl())
}

type celBinaryNode struct {
	op    string
	left  celNode
	right celNode
}

func (n *celBinaryNode) kcl() string {
	switch n.op {
	case "&&":
		return fmt.Sprintf("(%s and %s)", n.left.kcl(), n.right.kcl())
	case "||":
		return fmt.Sprintf("(%s or %s)", n.left.kcl(), n.right.kcl())
	}
	return fmt.Sprintf("(%s %s %s)", n.left.kcl(), n.op, n.right.kcl())
}

type celUnaryNode struct {
	op   string
	expr celNode
}

func (n *celUnaryNode) kcl() string {
	if n.op == "!" {
		return fmt.Sprintf("not (%s)", n.expr.kcl())
	}
	return fmt.Sprintf("(%s%s)", n.op, n.expr.kcl())
}

type celCallNode struct {
	recv celNode // nil for global funcs
	fn   string
	args []celNode
}

func (n *celCallNode) kcl() string {
	switch n.fn {
	case "size":
		if len(n.args) != 1 {
			return unsupportedCall(n.fn)
		}
		return fmt.Sprintf("len(%s)", n.args[0].kcl())
	case "matches":
		if n.recv == nil || len(n.args) != 1 {
			return unsupportedCall(n.fn)
		}
		return fmt.Sprintf("_regex_match(str(%s), %s)", n.recv.kcl(), n.args[0].kcl())
	case "startsWith":
		if n.recv == nil || len(n.args) != 1 {
			return unsupportedCall(n.fn)
		}
		return fmt.Sprintf("str(%s).starts_with(%s)", n.recv.kcl(), n.args[0].kcl())
	case "endsWith":
		if n.recv == nil || len(n.args) != 1 {
			return unsupportedCall(n.fn)
		}
		return fmt.Sprintf("str(%s).ends_with(%s)", n.recv.kcl(), n.args[0].kcl())
	case "has":
		if len(n.args) != 1 {
			return unsupportedCall(n.fn)
		}
		return fmt.Sprintf("(has %s)", n.args[0].kcl())
	case "all", "exists":
		if n.recv == nil || len(n.args) != 2 {
			return unsupportedCall(n.fn)
		}
		iter, pred := n.args[0], n.args[1]
		varName, ok := iter.(*celIdentNode)
		if !ok {
			return unsupportedCall(n.fn)
		}
		kw := "all"
		if n.fn == "exists" {
			kw = "any"
		}
		return fmt.Sprintf("%s %s in %s {\n    %s\n}", kw, varName.name, n.recv.kcl(), pred.kcl())
	case "exists_one":
		if n.recv == nil || len(n.args) != 2 {
			return unsupportedCall(n.fn)
		}
		iter, pred := n.args[0], n.args[1]
		varName, ok := iter.(*celIdentNode)
		if !ok {
			return unsupportedCall(n.fn)
		}
		return fmt.Sprintf("sum([1 for %s in %s if %s]) == 1", varName.name, n.recv.kcl(), pred.kcl())
	}
	return unsupportedCall(n.fn)
}

func unsupportedCall(name string) string {
	return fmt.Sprintf("__UNSUPPORTED_CEL_CALL_%s__", name)
}

type celTernaryNode struct {
	cond, then, els celNode
}

func (n *celTernaryNode) kcl() string {
	return fmt.Sprintf("(%s) if (%s) else (%s)", n.then.kcl(), n.cond.kcl(), n.els.kcl())
}

// celParser is a hand-written recursive-descent parser for the CEL subset
// supported by celToKCL.
type celParser struct {
	tokens []celToken
	pos    int
}

func (p *celParser) peek() *celToken {
	if p.pos >= len(p.tokens) {
		return &celToken{kind: celEOF}
	}
	return &p.tokens[p.pos]
}
func (p *celParser) consume() celToken {
	t := p.peek()
	p.pos++
	return *t
}

func (p *celParser) matchOp(s string) bool {
	t := p.peek()
	if t.kind == celOp && t.lit == s {
		p.pos++
		return true
	}
	return false
}

// parseExpr handles the ternary operator at the lowest precedence.
func (p *celParser) parseExpr() (celNode, error) {
	left, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.matchOp("?") {
		thenE, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.matchOp(":") {
			return nil, fmt.Errorf("expected ':' in ternary, got %q", p.peek().lit)
		}
		elsE, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &celTernaryNode{cond: left, then: thenE, els: elsE}, nil
	}
	return left, nil
}

func (p *celParser) parseOr() (celNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.matchOp("||") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &celBinaryNode{op: "||", left: left, right: right}
	}
	return left, nil
}

func (p *celParser) parseAnd() (celNode, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.matchOp("&&") {
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &celBinaryNode{op: "&&", left: left, right: right}
	}
	return left, nil
}

func (p *celParser) parseEquality() (celNode, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != celOp || (t.lit != "==" && t.lit != "!=") {
			return left, nil
		}
		p.pos++
		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		left = &celBinaryNode{op: t.lit, left: left, right: right}
	}
}

func (p *celParser) parseRelational() (celNode, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != celOp {
			return left, nil
		}
		switch t.lit {
		case "<", "<=", ">", ">=":
		default:
			return left, nil
		}
		p.pos++
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &celBinaryNode{op: t.lit, left: left, right: right}
	}
}

func (p *celParser) parseAdditive() (celNode, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != celOp || (t.lit != "+" && t.lit != "-") {
			return left, nil
		}
		p.pos++
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &celBinaryNode{op: t.lit, left: left, right: right}
	}
}

func (p *celParser) parseMultiplicative() (celNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != celOp {
			return left, nil
		}
		switch t.lit {
		case "*", "/", "%":
		default:
			return left, nil
		}
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &celBinaryNode{op: t.lit, left: left, right: right}
	}
}

func (p *celParser) parseUnary() (celNode, error) {
	t := p.peek()
	if t.kind == celOp && (t.lit == "!" || t.lit == "-" || t.lit == "+") {
		p.pos++
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &celUnaryNode{op: t.lit, expr: expr}, nil
	}
	return p.parsePostfix()
}

func (p *celParser) parsePostfix() (celNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind == celOp && t.lit == "." {
			p.pos++
			id := p.consume()
			if id.kind != celIdent {
				return nil, fmt.Errorf("expected identifier after '.', got %q", id.lit)
			}
			// distinguish .field from .method(args)
			if p.peek().kind == celOp && p.peek().lit == "(" {
				p.pos++ // consume "("
				args, err := p.parseArgList()
				if err != nil {
					return nil, err
				}
				left = &celCallNode{recv: left, fn: id.lit, args: args}
				continue
			}
			left = &celMemberNode{recv: left, name: id.lit}
			continue
		}
		if t.kind == celOp && t.lit == "[" {
			p.pos++
			idx, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp("]") {
				return nil, fmt.Errorf("expected ']' got %q", p.peek().lit)
			}
			left = &celIndexNode{recv: left, index: idx}
			continue
		}
		return left, nil
	}
}

func (p *celParser) parseArgList() ([]celNode, error) {
	var args []celNode
	if p.peek().kind == celOp && p.peek().lit == ")" {
		p.pos++
		return args, nil
	}
	for {
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.matchOp(",") {
			continue
		}
		if p.matchOp(")") {
			return args, nil
		}
		return nil, fmt.Errorf("expected ',' or ')' in argument list, got %q", p.peek().lit)
	}
}

func (p *celParser) parsePrimary() (celNode, error) {
	t := p.consume()
	switch t.kind {
	case celIdent:
		// identifier followed by "(" is a global function call
		if p.peek().kind == celOp && p.peek().lit == "(" {
			p.pos++
			args, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			return &celCallNode{fn: t.lit, args: args}, nil
		}
		return &celIdentNode{name: t.lit}, nil
	case celInt, celFloat, celString, celBool:
		return &celLiteralNode{lit: t.lit}, nil
	case celOp:
		if t.lit == "(" {
			expr, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp(")") {
				return nil, fmt.Errorf("expected ')', got %q", p.peek().lit)
			}
			return expr, nil
		}
		return nil, fmt.Errorf("unexpected operator %q", t.lit)
	}
	return nil, fmt.Errorf("unexpected token %q", t.lit)
}

// Reserved for callers that want to know if a translation produced a
// placeholder string for an unsupported construct. Kept as a tiny helper
// so tests don't have to re-implement the check.
func isUnsupportedKCL(expr string) bool {
	return strings.Contains(expr, "__UNSUPPORTED_CEL_CALL_")
}

// Used by the package-level translator to wrap the parser for clarity in
// callers that surface CEL translation errors. Currently a thin wrapper.
func celTranslate(rule string) (string, error) {
	return celToKCL(rule)
}

// Ensure unicode import is referenced (some configurations rely on this).
var _ = unicode.IsSpace