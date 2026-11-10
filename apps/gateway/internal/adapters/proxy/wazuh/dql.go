package wazuh

import (
	"fmt"
	"strings"
	"unicode"
)

// The supported DQL subset compiles to fixed clauses; query_string and raw DSL
// are never forwarded to OpenSearch. Quoted wildcard characters stay literal.
type dqlParser struct {
	pdqlParser
	depth, clauses int
}

func dqlToQuery(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len(raw) > maxFilterLength || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return nil, sourceRequestError("invalid_source_request", "DQL exceeds limits or contains control characters")
	}
	p := dqlParser{pdqlParser: pdqlParser{input: raw}}
	q, err := p.expression(false)
	p.skipSpaces()
	if err == nil && p.pos != len(raw) {
		err = p.failure("unexpected token")
	}
	return q, err
}
func (p *dqlParser) failure(message string) error {
	return sourceRequestError("invalid_source_request", fmt.Sprintf("Wazuh DQL: %s at position %d", message, p.pos+1))
}
func dqlBool(op string, a, b map[string]any) map[string]any {
	body := map[string]any{op: []any{a, b}}
	if op == "should" {
		body["minimum_should_match"] = 1
	}
	return map[string]any{"bool": body}
}
func (p *dqlParser) expression(and bool) (map[string]any, error) {
	var q map[string]any
	var err error
	if and {
		q, err = p.atom()
	} else {
		q, err = p.expression(true)
	}
	if err != nil {
		return nil, err
	}
	word, op := "or", "should"
	if and {
		word, op = "and", "filter"
	}
	for p.consumeKeyword(word) {
		var right map[string]any
		if and {
			right, err = p.atom()
		} else {
			right, err = p.expression(true)
		}
		if err != nil {
			return nil, err
		}
		q = dqlBool(op, q, right)
	}
	return q, nil
}
func (p *dqlParser) atom() (map[string]any, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 16 {
		return nil, p.failure("nesting exceeds 16")
	}
	if p.consumeKeyword("not") {
		q, err := p.atom()
		return map[string]any{"bool": map[string]any{"must_not": []any{q}}}, err
	}
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return nil, p.failure("incomplete condition")
	}
	if p.input[p.pos] == '(' {
		p.pos++
		q, err := p.expression(false)
		p.skipSpaces()
		if err != nil {
			return nil, err
		}
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, p.failure("expected closing parenthesis")
		}
		p.pos++
		return q, nil
	}
	p.clauses++
	if p.clauses > 128 {
		return nil, p.failure("too many clauses")
	}
	field, err := p.parseField()
	if err != nil {
		return nil, p.failure("expected field")
	}
	spec, ok := lookupField(field)
	if !ok {
		return nil, sourceRequestError("unsupported_filter_field", "Wazuh does not support field "+field)
	}
	p.skipSpaces()
	op := ""
	for _, candidate := range []string{">=", "<=", ">", "<", ":"} {
		if strings.HasPrefix(p.input[p.pos:], candidate) {
			op = candidate
			p.pos += len(candidate)
			break
		}
	}
	if op == "" {
		return nil, p.failure("expected ':' or a range operator")
	}
	p.skipSpaces()
	if op == ":" && p.pos < len(p.input) && p.input[p.pos] == '(' {
		p.pos++
		var query map[string]any
		join := "should"
		for {
			value, quoted, err := p.literal()
			if err != nil {
				return nil, err
			}
			q, err := dqlValue(spec, ":", value, quoted)
			if err != nil {
				return nil, err
			}
			if query == nil {
				query = q
			} else {
				query = dqlBool(join, query, q)
			}
			if p.consumeKeyword("or") {
				join = "should"
				continue
			}
			if p.consumeKeyword("and") {
				return nil, p.failure("field value lists support OR only")
			}
			p.skipSpaces()
			if p.pos >= len(p.input) || p.input[p.pos] != ')' {
				return nil, p.failure("expected closing value list")
			}
			p.pos++
			return query, nil
		}
	}
	value, quoted, err := p.literal()
	if err != nil {
		return nil, err
	}
	return dqlValue(spec, op, value, quoted)
}
func (p *dqlParser) literal() (string, bool, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return "", false, p.failure("expected value")
	}
	if p.input[p.pos] == '"' {
		v, err := p.parseString()
		return v, true, err
	}
	start := p.pos
	for p.pos < len(p.input) && !strings.ContainsRune(" ():\t\r\n", rune(p.input[p.pos])) {
		p.pos++
	}
	if start == p.pos {
		return "", false, p.failure("expected value")
	}
	value := p.input[start:p.pos]
	if strings.ContainsAny(value, "|;{}[]\\\"?~^<>=!") {
		return "", false, p.failure("unsupported value syntax")
	}
	return value, false, nil
}
func dqlValue(spec fieldSpec, op, value string, quoted bool) (map[string]any, error) {
	if op == ":" && !quoted && value == "*" {
		return map[string]any{"exists": map[string]any{"field": spec.Path}}, nil
	}
	if !validFieldLiteral(spec, value) {
		return nil, sourceRequestError("invalid_source_request", "DQL value must match field type; dates must be absolute")
	}
	if op != ":" {
		if spec.Type != fieldDate && spec.Type != fieldLong {
			return nil, sourceRequestError("unsupported_query_control", "DQL range requires a numeric or date field")
		}
		key := map[string]string{">": "gt", ">=": "gte", "<": "lt", "<=": "lte"}[op]
		return map[string]any{"range": map[string]any{spec.Path: map[string]any{key: coerceRangeValue(spec, value)}}}, nil
	}
	if !quoted && strings.Contains(value, "*") {
		if spec.Type != fieldKeyword || len(value) > 256 || strings.Count(value, "*") > 4 {
			return nil, sourceRequestError("invalid_source_request", "unsupported DQL wildcard")
		}
		return map[string]any{"wildcard": map[string]any{spec.Path: map[string]any{"value": value}}}, nil
	}
	return map[string]any{"term": map[string]any{spec.Path: coerceRangeValue(spec, value)}}, nil
}
