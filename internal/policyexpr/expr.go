package policyexpr

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type BoolExpr interface {
	Eval(vars map[string]any) (bool, error)
}

func ParseBoolExpr(s string) (BoolExpr, error) {
	toks, err := tokenize(s)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	n, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("表达式解析失败：多余内容 %q", p.peek().lit)
	}
	return n, nil
}

// ---- lexer ----

type tokKind int

const (
	tokEOF tokKind = iota
	tokIdent
	tokNumber
	tokString
	tokOp
	tokLParen
	tokRParen
)

type token struct {
	kind tokKind
	lit  string
}

func tokenize(s string) ([]token, error) {
	var out []token
	i := 0
	for i < len(s) {
		r := rune(s[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}

		// parens
		if s[i] == '(' {
			out = append(out, token{kind: tokLParen, lit: "("})
			i++
			continue
		}
		if s[i] == ')' {
			out = append(out, token{kind: tokRParen, lit: ")"})
			i++
			continue
		}

		// operators (2-char first)
		if i+1 < len(s) {
			op2 := s[i : i+2]
			switch op2 {
			case "&&", "||", "==", "!=", ">=", "<=":
				out = append(out, token{kind: tokOp, lit: op2})
				i += 2
				continue
			}
		}
		// 1-char operators
		switch s[i] {
		case '=':
			out = append(out, token{kind: tokOp, lit: s[i : i+1]})
			i++
			continue
		case '>', '<':
			out = append(out, token{kind: tokOp, lit: s[i : i+1]})
			i++
			continue
		}

		// quoted string
		if s[i] == '"' || s[i] == '\'' {
			quote := s[i]
			i++
			start := i
			for i < len(s) && s[i] != quote {
				// v1：不支持复杂转义；需要时再扩展。
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("字符串字面量未闭合")
			}
			out = append(out, token{kind: tokString, lit: s[start:i]})
			i++ // consume quote
			continue
		}

		// number (allow leading -)
		if s[i] == '-' || (s[i] >= '0' && s[i] <= '9') {
			start := i
			if s[i] == '-' {
				i++
				if i >= len(s) || s[i] < '0' || s[i] > '9' {
					return nil, fmt.Errorf("非法数字字面量")
				}
			}
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			out = append(out, token{kind: tokNumber, lit: s[start:i]})
			continue
		}

		// ident
		if isIdentStart(s[i]) {
			start := i
			i++
			for i < len(s) && isIdentPart(s[i]) {
				i++
			}
			out = append(out, token{kind: tokIdent, lit: s[start:i]})
			continue
		}

		return nil, fmt.Errorf("无法识别的字符: %q", s[i])
	}

	out = append(out, token{kind: tokEOF, lit: ""})
	return out, nil
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

// ---- parser ----

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token {
	if p.pos >= len(p.toks) {
		return token{kind: tokEOF}
	}
	return p.toks[p.pos]
}

func (p *parser) next() token {
	t := p.peek()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *parser) expect(kind tokKind, lit string) error {
	t := p.next()
	if t.kind != kind {
		return fmt.Errorf("表达式解析失败：期待 %v，实际 %v", kind, t.kind)
	}
	if lit != "" && t.lit != lit {
		return fmt.Errorf("表达式解析失败：期待 %q，实际 %q", lit, t.lit)
	}
	return nil
}

func (p *parser) parseExpr() (node, error) { return p.parseOr() }

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOp && p.peek().lit == "||" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orNode{left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOp && p.peek().lit == "&&" {
		p.next()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = andNode{left: left, right: right}
	}
	return left, nil
}

func (p *parser) parsePrimary() (node, error) {
	if p.peek().kind == tokLParen {
		p.next()
		n, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokRParen, ")"); err != nil {
			return nil, err
		}
		return n, nil
	}
	return p.parseComparison()
}

func (p *parser) parseComparison() (node, error) {
	id := p.next()
	if id.kind != tokIdent {
		return nil, fmt.Errorf("表达式解析失败：期待标识符，实际 %q", id.lit)
	}

	op := p.next()
	if op.kind != tokOp || !isCmpOp(op.lit) {
		return nil, fmt.Errorf("表达式解析失败：期待比较运算符，实际 %q", op.lit)
	}
	opLit := op.lit
	if opLit == "=" {
		opLit = "=="
	}

	litTok := p.next()
	lit, err := parseLiteral(litTok)
	if err != nil {
		return nil, err
	}

	return cmpNode{ident: id.lit, op: opLit, lit: lit}, nil
}

func isCmpOp(op string) bool {
	switch op {
	case "==", "=", "!=", ">=", "<=", ">", "<":
		return true
	default:
		return false
	}
}

// ---- AST & eval ----

type node interface {
	BoolExpr
}

type andNode struct {
	left  node
	right node
}

func (n andNode) Eval(vars map[string]any) (bool, error) {
	a, err := n.left.Eval(vars)
	if err != nil {
		return false, err
	}
	if !a {
		return false, nil
	}
	return n.right.Eval(vars)
}

type orNode struct {
	left  node
	right node
}

func (n orNode) Eval(vars map[string]any) (bool, error) {
	a, err := n.left.Eval(vars)
	if err != nil {
		return false, err
	}
	if a {
		return true, nil
	}
	return n.right.Eval(vars)
}

type literalKind int

const (
	litInt literalKind = iota
	litBool
	litString
)

type literal struct {
	kind literalKind
	i    int64
	b    bool
	s    string
}

func parseLiteral(t token) (literal, error) {
	switch t.kind {
	case tokNumber:
		n, err := strconv.ParseInt(t.lit, 10, 64)
		if err != nil {
			return literal{}, fmt.Errorf("非法数字字面量: %q", t.lit)
		}
		return literal{kind: litInt, i: n}, nil
	case tokString:
		return literal{kind: litString, s: t.lit}, nil
	case tokIdent:
		switch strings.ToLower(t.lit) {
		case "true":
			return literal{kind: litBool, b: true}, nil
		case "false":
			return literal{kind: litBool, b: false}, nil
		default:
			// 允许不带引号的字符串字面量（便于写配置），但仅支持 == / !=。
			return literal{kind: litString, s: t.lit}, nil
		}
	default:
		return literal{}, fmt.Errorf("非法字面量: %q", t.lit)
	}
}

type cmpNode struct {
	ident string
	op    string
	lit   literal
}

func (n cmpNode) Eval(vars map[string]any) (bool, error) {
	raw, ok := vars[n.ident]
	if !ok {
		return false, fmt.Errorf("未知变量: %s", n.ident)
	}

	switch n.lit.kind {
	case litInt:
		lv, err := asInt64(raw)
		if err != nil {
			return false, fmt.Errorf("变量 %s 不是整数: %w", n.ident, err)
		}
		return cmpInt(lv, n.op, n.lit.i)
	case litBool:
		lv, err := asBool(raw)
		if err != nil {
			return false, fmt.Errorf("变量 %s 不是布尔: %w", n.ident, err)
		}
		return cmpBool(lv, n.op, n.lit.b)
	case litString:
		lv, err := asString(raw)
		if err != nil {
			return false, fmt.Errorf("变量 %s 不是字符串: %w", n.ident, err)
		}
		return cmpString(lv, n.op, n.lit.s)
	default:
		return false, errors.New("未知字面量类型")
	}
}

func asInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int:
		return int64(x), nil
	case int8:
		return int64(x), nil
	case int16:
		return int64(x), nil
	case int32:
		return int64(x), nil
	case int64:
		return x, nil
	case uint:
		return int64(x), nil
	case uint8:
		return int64(x), nil
	case uint16:
		return int64(x), nil
	case uint32:
		return int64(x), nil
	case uint64:
		if x > uint64(^uint64(0)>>1) {
			return 0, fmt.Errorf("uint64 溢出")
		}
		return int64(x), nil
	case float64:
		if x != float64(int64(x)) {
			return 0, fmt.Errorf("float64 非整数值")
		}
		return int64(x), nil
	case float32:
		f := float64(x)
		if f != float64(int64(f)) {
			return 0, fmt.Errorf("float32 非整数值")
		}
		return int64(f), nil
	default:
		return 0, fmt.Errorf("类型 %T 不支持作为整数", v)
	}
}

func asBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	default:
		return false, fmt.Errorf("类型 %T 不支持作为布尔", v)
	}
}

func asString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	default:
		return "", fmt.Errorf("类型 %T 不支持作为字符串", v)
	}
}

func cmpInt(a int64, op string, b int64) (bool, error) {
	switch op {
	case "==":
		return a == b, nil
	case "!=":
		return a != b, nil
	case ">=":
		return a >= b, nil
	case "<=":
		return a <= b, nil
	case ">":
		return a > b, nil
	case "<":
		return a < b, nil
	default:
		return false, fmt.Errorf("整数比较不支持运算符 %q", op)
	}
}

func cmpBool(a bool, op string, b bool) (bool, error) {
	switch op {
	case "==":
		return a == b, nil
	case "!=":
		return a != b, nil
	default:
		return false, fmt.Errorf("布尔比较仅支持 == / !=，实际 %q", op)
	}
}

func cmpString(a string, op string, b string) (bool, error) {
	switch op {
	case "==":
		return a == b, nil
	case "!=":
		return a != b, nil
	default:
		return false, fmt.Errorf("字符串比较仅支持 == / !=，实际 %q", op)
	}
}
