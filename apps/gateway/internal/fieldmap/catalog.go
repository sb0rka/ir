package fieldmap

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

//go:embed mappings.json
var document []byte

type Field struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Label     string   `json:"label"`
	Operators []string `json:"operators"`
}
type Path struct {
	Path            string `json:"path"`
	CaseInsensitive bool   `json:"case_insensitive,omitempty"`
}
type Source struct {
	Time     string `json:"time"`
	Strategy string `json:"strategy"`
	Host     []Path `json:"host"`
	IP       []Path `json:"ip"`
}
type Catalog struct {
	Version int               `json:"version"`
	Fields  []Field           `json:"fields"`
	Sources map[string]Source `json:"sources"`
}
type CommonFilter struct {
	Hosts []string `json:"hosts,omitempty"`
	IPs   []string `json:"ips,omitempty"`
}

var pathPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]*$`)
var current, loadError = Decode(document)

func Decode(raw []byte) (*Catalog, error) {
	// Duplicate keys must fail rather than silently replacing a mapping.
	if err := uniqueKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var value Catalog
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	if d.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("trailing field map data")
	}
	if value.Version != 1 || len(value.Fields) != 4 || len(value.Sources) == 0 {
		return nil, fmt.Errorf("invalid field map version or fields")
	}
	expected := map[string]string{"time": "datetime", "source": "string", "host": "string", "ip": "ip"}
	seen := map[string]bool{}
	for _, f := range value.Fields {
		if expected[f.Name] != f.Type || seen[f.Name] || f.Label == "" || len(f.Operators) == 0 {
			return nil, fmt.Errorf("invalid common field %q", f.Name)
		}
		allowedOps := map[string][]string{"time": {"range"}, "source": {"in"}, "host": {"=", "in"}, "ip": {"=", "in"}}
		ops := map[string]bool{}
		for _, op := range f.Operators {
			if !slices.Contains(allowedOps[f.Name], op) || ops[op] {
				return nil, fmt.Errorf("invalid common operator for %s", f.Name)
			}
			ops[op] = true
		}
		seen[f.Name] = true
	}
	for code, s := range value.Sources {
		if !pathPattern.MatchString(code) || !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_./-]*$`).MatchString(s.Time) || (s.Strategy != "native" && s.Strategy != "entities") {
			return nil, fmt.Errorf("invalid source mapping %q", code)
		}
		for _, paths := range [][]Path{s.Host, s.IP} {
			seenPaths := map[string]bool{}
			for _, p := range paths {
				if !pathPattern.MatchString(p.Path) || seenPaths[p.Path] {
					return nil, fmt.Errorf("invalid mapping path for %s", code)
				}
				seenPaths[p.Path] = true
			}
			if s.Strategy == "native" && len(paths) == 0 {
				return nil, fmt.Errorf("missing native paths for %s", code)
			}
		}
	}
	return &value, nil
}
func uniqueKeys(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for d.More() {
		if delimiter == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate field map key")
			}
			seen[name] = true
		}
		if err := uniqueKeys(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
func Default() (*Catalog, error) { return current, loadError }
func Hash() string               { sum := sha256.Sum256(document); return hex.EncodeToString(sum[:]) }
func Paths(source, kind string) ([]Path, error) {
	if loadError != nil {
		return nil, loadError
	}
	s, ok := current.Sources[source]
	if !ok {
		return nil, fmt.Errorf("source has no common field mapping")
	}
	if kind == "host" || kind == "hostname" {
		return append([]Path(nil), s.Host...), nil
	}
	if kind == "ip" {
		return append([]Path(nil), s.IP...), nil
	}
	return nil, fmt.Errorf("not a mapped entity type")
}
func Normalize(filter CommonFilter) (CommonFilter, error) {
	result := CommonFilter{}
	for _, item := range []struct {
		kind   string
		values []string
		target *[]string
	}{{"host", filter.Hosts, &result.Hosts}, {"ip", filter.IPs, &result.IPs}} {
		if len(item.values) > 100 {
			return result, &domain.RequestError{Code: "invalid_common_filter", Message: "common filter allows at most 100 values per field"}
		}
		seen := map[string]bool{}
		for _, raw := range item.values {
			if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
				return result, &domain.RequestError{Code: "invalid_common_filter", Message: "invalid common " + item.kind + " value"}
			}
			value := domain.CanonicalValue(item.kind, raw)
			if value == "" || len(value) > 253 || strings.IndexFunc(value, unicode.IsControl) >= 0 || item.kind == "ip" && net.ParseIP(value) == nil {
				return result, &domain.RequestError{Code: "invalid_common_filter", Message: "invalid common " + item.kind + " value"}
			}
			if item.kind == "host" {
				for _, c := range value {
					if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
						return result, &domain.RequestError{Code: "invalid_common_filter", Message: "invalid common host value"}
					}
				}
			}
			if !seen[value] {
				*item.target = append(*item.target, value)
				seen[value] = true
			}
		}
	}
	return result, nil
}
func (filter CommonFilter) Empty() bool { return len(filter.Hosts) == 0 && len(filter.IPs) == 0 }
func (filter CommonFilter) Matches(event domain.Event) bool {
	for _, item := range []struct {
		kind   string
		values []string
	}{{"host", filter.Hosts}, {"ip", filter.IPs}} {
		if len(item.values) == 0 {
			continue
		}
		matched := false
		for _, m := range event.Entities {
			if m.Type != item.kind {
				continue
			}
			for _, v := range item.values {
				if domain.CanonicalValue(item.kind, m.Value) == v {
					matched = true
				}
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// ValidateNativePaths joins declarative mappings with the adapter-owned field
// vocabulary. A typo must fail at startup rather than weaken an identity query.
func ValidateNativePaths(code string, fields []string) error {
	c, err := Default()
	if err != nil {
		return err
	}
	source, ok := c.Sources[code]
	if !ok {
		return fmt.Errorf("source %s has no common mapping", code)
	}
	if source.Strategy == "entities" {
		return nil
	}
	if !slices.Contains(fields, source.Time) {
		return fmt.Errorf("source %s has an unsupported time mapping %s", code, source.Time)
	}
	for _, paths := range [][]Path{source.Host, source.IP} {
		for _, p := range paths {
			if !slices.Contains(fields, p.Path) {
				return fmt.Errorf("source %s has an unsupported identity mapping %s", code, p.Path)
			}
		}
	}
	return nil
}
