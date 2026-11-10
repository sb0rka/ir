package maxpatrol

import (
	"fmt"
	"slices"
	"strings"
	"text/scanner"
	"unicode"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
)

// The explicit PDQL mode accepts predicates, never pipeline stages or functions.
// Legacy requests keep the pre-existing bounded validator.
type predicateParser struct {
	fields    map[string]capability.EventField
	scanError bool
	s         scanner.Scanner
	token     rune
	depth     int
}

func validateNativePredicate(text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	p := predicateParser{fields: map[string]capability.EventField{}}
	for _, field := range (&Provider{}).EventQueryDescription().Fields {
		p.fields[field.Name] = field
	}
	p.s.Init(strings.NewReader(text))
	p.s.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanInts | scanner.ScanFloats
	p.s.IsIdentRune = func(r rune, i int) bool {
		return unicode.IsLetter(r) || r == '_' || i > 0 && (unicode.IsDigit(r) || r == '.')
	}
	p.s.Error = func(_ *scanner.Scanner, _ string) { p.scanError = true }
	p.next()
	if err := p.expression(false); err != nil {
		return err
	}
	if p.scanError {
		return p.fail("invalid literal")
	}
	if p.token != scanner.EOF {
		return p.fail("unexpected token")
	}
	return nil
}
func (p *predicateParser) next() { p.token = p.s.Scan() }
func (p *predicateParser) keyword(word string) bool {
	return p.token == scanner.Ident && strings.EqualFold(p.s.TokenText(), word)
}
func (p *predicateParser) fail(message string) error {
	return sourceRequestError("unsupported_query_control", fmt.Sprintf("PT PDQL: %s near %q", message, p.s.TokenText()))
}
func (p *predicateParser) expression(and bool) error {
	var err error
	if and {
		err = p.atom()
	} else {
		err = p.expression(true)
	}
	if err != nil {
		return err
	}
	word := "or"
	if and {
		word = "and"
	}
	for p.keyword(word) {
		p.next()
		if and {
			err = p.atom()
		} else {
			err = p.expression(true)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func (p *predicateParser) atom() error {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 16 {
		return p.fail("nesting exceeds 16")
	}
	if p.keyword("not") {
		p.next()
		return p.atom()
	}
	if p.token == '(' {
		p.next()
		if err := p.expression(false); err != nil {
			return err
		}
		if p.token != ')' {
			return p.fail("expected closing parenthesis")
		}
		p.next()
		return nil
	}
	if p.token != scanner.Ident {
		return p.fail("expected field")
	}
	field := p.s.TokenText()
	if _, ok := p.fields[field]; !ok {
		return sourceRequestError("unsupported_filter_field", "PT PDQL does not support field "+field)
	}
	p.next()
	if p.keyword("contains") {
		if !slices.Contains(p.fields[field].Operators, "contains") {
			return p.fail("contains is not supported for " + field)
		}
		p.next()
		if p.token != scanner.String {
			return p.fail("contains requires a quoted string")
		}
		p.next()
		return nil
	}
	op := p.token
	operator := string(op)
	if op != '=' && op != '!' && op != '>' && op != '<' {
		return p.fail("unsupported operator")
	}
	p.next()
	if p.token == '=' {
		if op == '=' {
			return p.fail("use = rather than ==")
		}
		operator += "="
		p.next()
	} else if op == '!' {
		return p.fail("expected !=")
	}
	if !slices.Contains(p.fields[field].Operators, operator) {
		return p.fail("operator " + operator + " is not supported for " + field)
	}
	if p.keyword("null") && operator != "=" && operator != "!=" {
		return p.fail("null requires = or != for " + field)
	}
	if p.token == '-' || p.token == '+' {
		p.next()
		if p.token != scanner.Int && p.token != scanner.Float {
			return p.fail("expected number")
		}
	}
	if p.token != scanner.String && p.token != scanner.Int && p.token != scanner.Float && !p.keyword("null") && !p.keyword("true") && !p.keyword("false") {
		return p.fail("expected literal")
	}
	p.next()
	return nil
}
