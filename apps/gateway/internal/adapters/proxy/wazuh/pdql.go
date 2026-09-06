package wazuh

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const maxFilterLength = 4096

type pdqlNode interface {
	toQuery() (map[string]any, error)
}

type boolNode struct {
	Op    string // and | or
	Left  pdqlNode
	Right pdqlNode
}

type notNode struct {
	Child pdqlNode
}

type predicateNode struct {
	Field string
	Op    string
	Value string
	Values []string
}

func parsePDQLFilter(raw string) (pdqlNode, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if len(raw) > maxFilterLength {
		return nil, &RequestError{Operation: "event search", Message: "filter is too long"}
	}
	if strings.ContainsAny(raw, "|;\r\n\x00") || strings.Contains(raw, "--") || strings.Contains(raw, "/*") || strings.Contains(raw, "*/") {
		return nil, &RequestError{Operation: "event search", Message: "filter must be a predicate without query pipelines or comments"}
	}
	parser := &pdqlParser{input: raw}
	node, err := parser.parseOr()
	if err != nil {
		return nil, err
	}
	parser.skipSpaces()
	if parser.pos < len(parser.input) {
		return nil, &RequestError{Operation: "event search", Message: "filter has trailing tokens"}
	}
	return node, nil
}

type pdqlParser struct {
	input string
	pos   int
}

func (p *pdqlParser) skipSpaces() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *pdqlParser) peekKeyword(word string) bool {
	p.skipSpaces()
	if !strings.HasPrefix(strings.ToLower(p.input[p.pos:]), word) {
		return false
	}
	end := p.pos + len(word)
	if end < len(p.input) {
		next := p.input[end]
		if isIdentPart(next) {
			return false
		}
	}
	return true
}

func (p *pdqlParser) consumeKeyword(word string) bool {
	if !p.peekKeyword(word) {
		return false
	}
	p.pos += len(word)
	return true
}

func (p *pdqlParser) parseOr() (pdqlNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.consumeKeyword("or") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &boolNode{Op: "or", Left: left, Right: right}
	}
	return left, nil
}

func (p *pdqlParser) parseAnd() (pdqlNode, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.consumeKeyword("and") {
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &boolNode{Op: "and", Left: left, Right: right}
	}
	return left, nil
}

func (p *pdqlParser) parseNot() (pdqlNode, error) {
	if p.consumeKeyword("not") {
		child, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &notNode{Child: child}, nil
	}
	return p.parsePrimary()
}

func (p *pdqlParser) parsePrimary() (pdqlNode, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return nil, &RequestError{Operation: "event search", Message: "filter is incomplete"}
	}
	if p.input[p.pos] == '(' {
		p.pos++
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, &RequestError{Operation: "event search", Message: "filter parentheses are unbalanced"}
		}
		p.pos++
		return node, nil
	}
	return p.parsePredicate()
}

func (p *pdqlParser) parsePredicate() (pdqlNode, error) {
	field, err := p.parseField()
	if err != nil {
		return nil, err
	}
	if _, ok := lookupField(field); !ok {
		return nil, &RequestError{Operation: "event search", Message: "filter contains an unsupported field"}
	}
	p.skipSpaces()
	if p.consumeKeyword("is") {
		negated := p.consumeKeyword("not")
		if !p.consumeKeyword("null") {
			return nil, &RequestError{Operation: "event search", Message: "filter null predicate is invalid"}
		}
		op := "is_null"
		if negated {
			op = "is_not_null"
		}
		return &predicateNode{Field: field, Op: op}, nil
	}
	if p.consumeKeyword("contains") {
		value, err := p.parseString()
		if err != nil {
			return nil, err
		}
		return &predicateNode{Field: field, Op: "contains", Value: value}, nil
	}
	if p.consumeKeyword("in") {
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != '(' {
			return nil, &RequestError{Operation: "event search", Message: "filter in-list is invalid"}
		}
		p.pos++
		values := make([]string, 0, 4)
		for {
			value, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			values = append(values, value)
			p.skipSpaces()
			if p.pos < len(p.input) && p.input[p.pos] == ',' {
				p.pos++
				continue
			}
			break
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, &RequestError{Operation: "event search", Message: "filter in-list is invalid"}
		}
		p.pos++
		return &predicateNode{Field: field, Op: "in", Values: values}, nil
	}
	op, err := p.parseOperator()
	if err != nil {
		return nil, err
	}
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return &predicateNode{Field: field, Op: op, Value: value}, nil
}

func (p *pdqlParser) parseField() (string, error) {
	p.skipSpaces()
	start := p.pos
	if p.pos >= len(p.input) || !isIdentStart(p.input[p.pos]) {
		return "", &RequestError{Operation: "event search", Message: "filter field is invalid"}
	}
	p.pos++
	for p.pos < len(p.input) && isIdentPart(p.input[p.pos]) {
		p.pos++
	}
	return p.input[start:p.pos], nil
}

func (p *pdqlParser) parseOperator() (string, error) {
	p.skipSpaces()
	candidates := []string{"!=", ">=", "<=", "=", ">", "<"}
	for _, op := range candidates {
		if strings.HasPrefix(p.input[p.pos:], op) {
			p.pos += len(op)
			return op, nil
		}
	}
	return "", &RequestError{Operation: "event search", Message: "filter operator is invalid"}
}

func (p *pdqlParser) parseValue() (string, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return "", &RequestError{Operation: "event search", Message: "filter value is missing"}
	}
	if p.input[p.pos] == '"' {
		return p.parseString()
	}
	start := p.pos
	if p.input[p.pos] == '-' {
		p.pos++
	}
	digits := false
	for p.pos < len(p.input) && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9' || p.input[p.pos] == '.') {
		digits = true
		p.pos++
	}
	if digits {
		return p.input[start:p.pos], nil
	}
	p.pos = start
	if p.consumeKeyword("true") {
		return "true", nil
	}
	if p.consumeKeyword("false") {
		return "false", nil
	}
	return "", &RequestError{Operation: "event search", Message: "filter value is invalid"}
}

func (p *pdqlParser) parseString() (string, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) || p.input[p.pos] != '"' {
		return "", &RequestError{Operation: "event search", Message: "filter string is invalid"}
	}
	p.pos++
	var builder strings.Builder
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '\\' {
			p.pos++
			if p.pos >= len(p.input) {
				return "", &RequestError{Operation: "event search", Message: "filter string escape is invalid"}
			}
			builder.WriteByte(p.input[p.pos])
			p.pos++
			continue
		}
		if ch == '"' {
			p.pos++
			return builder.String(), nil
		}
		builder.WriteByte(ch)
		p.pos++
	}
	return "", &RequestError{Operation: "event search", Message: "filter quotes are unbalanced"}
}

func isIdentStart(value byte) bool {
	return value == '_' || (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}

func isIdentPart(value byte) bool {
	return isIdentStart(value) || value == '.' || (value >= '0' && value <= '9')
}

func (node *boolNode) toQuery() (map[string]any, error) {
	left, err := node.Left.toQuery()
	if err != nil {
		return nil, err
	}
	right, err := node.Right.toQuery()
	if err != nil {
		return nil, err
	}
	key := "filter"
	if node.Op == "or" {
		return map[string]any{"bool": map[string]any{
			"should":               []any{left, right},
			"minimum_should_match": 1,
		}}, nil
	}
	return map[string]any{"bool": map[string]any{key: []any{left, right}}}, nil
}

func (node *notNode) toQuery() (map[string]any, error) {
	child, err := node.Child.toQuery()
	if err != nil {
		return nil, err
	}
	return map[string]any{"bool": map[string]any{"must_not": []any{child}}}, nil
}

func (node *predicateNode) toQuery() (map[string]any, error) {
	spec, ok := lookupField(node.Field)
	if !ok {
		return nil, &RequestError{Operation: "event search", Message: "filter contains an unsupported field"}
	}
	switch {
	case node.Op == "is_null":
		return map[string]any{"bool": map[string]any{"must_not": []any{map[string]any{"exists": map[string]any{"field": existsField(spec)}}}}}, nil
	case node.Op == "is_not_null":
		return map[string]any{"exists": map[string]any{"field": existsField(spec)}}, nil
	case spec.CorrelationType:
		return correlationTypeQuery(node)
	case node.Op == "contains":
		if spec.Type != fieldKeyword || len(node.Value) == 0 || len(node.Value) > 256 {
			return nil, &RequestError{Operation: "event search", Message: "contains is not supported for this field"}
		}
		clause := map[string]any{"value": "*" + escapeWildcard(node.Value) + "*", "case_insensitive": true}
		query := map[string]any{"wildcard": map[string]any{spec.Path: clause}}
		return wrapCorrelationName(spec, query), nil
	case node.Op == "in":
		return termsQuery(spec, node.Values)
	case node.Op == "=":
		return termQuery(spec, node.Value, false)
	case node.Op == "!=":
		return termQuery(spec, node.Value, true)
	case node.Op == ">" || node.Op == ">=" || node.Op == "<" || node.Op == "<=":
		if spec.Type != fieldLong && spec.Type != fieldDate {
			return nil, &RequestError{Operation: "event search", Message: "range operators require a numeric or date field"}
		}
		rangeBody := map[string]any{}
		switch node.Op {
		case ">":
			rangeBody["gt"] = coerceRangeValue(spec, node.Value)
		case ">=":
			rangeBody["gte"] = coerceRangeValue(spec, node.Value)
		case "<":
			rangeBody["lt"] = coerceRangeValue(spec, node.Value)
		case "<=":
			rangeBody["lte"] = coerceRangeValue(spec, node.Value)
		}
		return map[string]any{"range": map[string]any{spec.Path: rangeBody}}, nil
	default:
		return nil, &RequestError{Operation: "event search", Message: "filter operator is invalid"}
	}
}

func existsField(spec fieldSpec) string {
	if spec.DocumentID {
		return "_id"
	}
	if spec.CorrelationType {
		return "rule.frequency"
	}
	return spec.Path
}

func wrapCorrelationName(spec fieldSpec, query map[string]any) map[string]any {
	if !spec.CorrelationName {
		return query
	}
	return map[string]any{"bool": map[string]any{
		"filter": []any{
			query,
			map[string]any{"exists": map[string]any{"field": "rule.frequency"}},
		},
	}}
}

func termQuery(spec fieldSpec, value string, negate bool) (map[string]any, error) {
	if spec.DocumentID {
		query := map[string]any{"ids": map[string]any{"values": []string{value}}}
		if negate {
			return map[string]any{"bool": map[string]any{"must_not": []any{query}}}, nil
		}
		return query, nil
	}
	body := map[string]any{"value": value}
	if spec.CaseInsensitive {
		body["case_insensitive"] = true
	}
	query := map[string]any{"term": map[string]any{spec.Path: body}}
	query = wrapCorrelationName(spec, query)
	if negate {
		return map[string]any{"bool": map[string]any{"must_not": []any{query}}}, nil
	}
	return query, nil
}

func termsQuery(spec fieldSpec, values []string) (map[string]any, error) {
	if len(values) == 0 {
		return nil, &RequestError{Operation: "event search", Message: "filter in-list is empty"}
	}
	if spec.DocumentID {
		return map[string]any{"ids": map[string]any{"values": values}}, nil
	}
	query := map[string]any{"terms": map[string]any{spec.Path: values}}
	return wrapCorrelationName(spec, query), nil
}

func correlationTypeQuery(node *predicateNode) (map[string]any, error) {
	value := strings.ToLower(strings.TrimSpace(node.Value))
	if node.Op == "in" && len(node.Values) == 1 {
		value = strings.ToLower(strings.TrimSpace(node.Values[0]))
	}
	if node.Op != "=" && !(node.Op == "in" && len(node.Values) == 1) {
		return nil, &RequestError{Operation: "event search", Message: "correlation_type only supports equality"}
	}
	exists := map[string]any{"exists": map[string]any{"field": "rule.frequency"}}
	switch value {
	case "wazuh_frequency":
		return exists, nil
	case "wazuh_rule":
		return map[string]any{"bool": map[string]any{"must_not": []any{exists}}}, nil
	default:
		return nil, &RequestError{Operation: "event search", Message: "correlation_type value is invalid"}
	}
}

func coerceRangeValue(spec fieldSpec, raw string) any {
	if spec.Type == fieldLong {
		if number, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return number
		}
	}
	return raw
}

func escapeWildcard(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`)
	return replacer.Replace(value)
}

func filterToQuery(raw string) (map[string]any, error) {
	node, err := parsePDQLFilter(raw)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}
	query, err := node.toQuery()
	if err != nil {
		return nil, err
	}
	if query == nil {
		return nil, fmt.Errorf("empty query")
	}
	return query, nil
}
