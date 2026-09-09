package ptnad

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unicode"
)

// Compile only the reviewed predicate grammar. Vendor pipelines and identifiers
// never pass through unparsed; nested tables are selected by the field allowlist.
func compileFilter(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", nil
	}
	if len(input) > 4096 {
		return "", invalidRequest("NAD filter exceeds 4096 bytes")
	}
	p := filterParser{input: input}
	value, err := p.expression(0)
	p.space()
	if err != nil || p.pos != len(input) {
		return "", invalidRequest("invalid or unsupported NAD filter")
	}
	return value, nil
}

type filterParser struct {
	input string
	pos   int
}

func (p *filterParser) space() {
	for p.pos < len(p.input) && p.input[p.pos] == ' ' {
		p.pos++
	}
}
func (p *filterParser) take(s string) bool {
	p.space()
	if strings.HasPrefix(p.input[p.pos:], s) {
		p.pos += len(s)
		return true
	}
	return false
}
func (p *filterParser) boolean(word, symbol string) bool {
	p.space()
	if p.take(symbol) {
		return true
	}
	end := p.pos + len(word)
	if end <= len(p.input) && strings.EqualFold(p.input[p.pos:end], word) && (end == len(p.input) || p.input[end] == ' ' || p.input[end] == '(') {
		p.pos = end
		return true
	}
	return false
}
func (p *filterParser) expression(depth int) (string, error) {
	left, err := p.conjunction(depth)
	if err != nil {
		return "", err
	}
	for p.boolean("or", "||") {
		right, err := p.conjunction(depth)
		if err != nil {
			return "", err
		}
		left = "(" + left + " OR " + right + ")"
	}
	return left, nil
}
func (p *filterParser) conjunction(depth int) (string, error) {
	left, err := p.atom(depth)
	if err != nil {
		return "", err
	}
	for p.boolean("and", "&&") {
		right, err := p.atom(depth)
		if err != nil {
			return "", err
		}
		left = "(" + left + " AND " + right + ")"
	}
	return left, nil
}
func (p *filterParser) atom(depth int) (string, error) {
	if depth > 16 {
		return "", fmt.Errorf("nesting limit")
	}
	if p.take("(") {
		value, err := p.expression(depth + 1)
		if err != nil || !p.take(")") {
			return "", fmt.Errorf("parentheses")
		}
		return "(" + value + ")", nil
	}
	p.space()
	start := p.pos
	for p.pos < len(p.input) && (p.input[p.pos] >= 'a' && p.input[p.pos] <= 'z' || p.input[p.pos] == '.' || p.input[p.pos] == '_') {
		p.pos++
	}
	field := p.input[start:p.pos]
	table, column, kind := "", field, "text"
	switch field {
	case "src.ip", "dst.ip", "host.ip":
		kind = "ip"
	case "host.port":
		kind = "port"
	case "app_proto":
	case "alert.msg":
		table, column = "alert", "msg"
	case "alert.pr":
		table, column, kind = "alert", "pr", "priority"
	case "files.filename":
		table, column = "files", "filename"
	case "smb.rqs.create.filename":
		table, column = "smb", "rqs.create.filename"
	default:
		return "", fmt.Errorf("field")
	}
	op := "=="
	if !p.take("==") {
		if !p.take("~") || kind != "text" {
			return "", fmt.Errorf("operator")
		}
		op = "~"
	}
	p.space()
	start = p.pos
	value := ""
	if p.take("\"") {
		start = p.pos - 1
		escaped := false
		closed := false
		for p.pos < len(p.input) {
			c := p.input[p.pos]
			p.pos++
			if !escaped && c == '"' {
				closed = true
				break
			}
			if !escaped && c == '\\' {
				escaped = true
			} else {
				escaped = false
			}
		}
		if !closed {
			return "", fmt.Errorf("string")
		}
		var err error
		value, err = strconv.Unquote(p.input[start:p.pos])
		if err != nil {
			return "", err
		}
	} else {
		for p.pos < len(p.input) && (p.input[p.pos] >= '0' && p.input[p.pos] <= '9' || strings.ContainsRune("abcdefABCDEF.:", rune(p.input[p.pos]))) {
			p.pos++
		}
		value = p.input[start:p.pos]
		if kind == "text" {
			return "", fmt.Errorf("quoted string required")
		}
	}
	if value == "" || len(value) > 1024 || strings.ContainsAny(value, "'\\") || strings.ContainsFunc(value, unicode.IsControl) {
		return "", fmt.Errorf("value")
	}
	literal := "'" + value + "'"
	switch kind {
	case "ip":
		if _, err := netip.ParseAddr(value); err != nil {
			return "", err
		}
	case "port", "priority":
		n, err := strconv.Atoi(value)
		max := 65535
		if kind == "priority" {
			max = 255
		}
		if err != nil || n < 0 || n > max {
			return "", fmt.Errorf("number")
		}
		literal = strconv.Itoa(n)
	}
	predicate := "'" + column + "' " + op + " " + literal
	if table != "" {
		predicate = "EXISTS (SELECT * FROM '" + table + "' WHERE " + predicate + ")"
	}

	return "(" + predicate + ")", nil
}

func bqlAnd(predicate string) string {
	if predicate == "" {
		return ""
	}
	return "AND " + predicate
}
