package sources

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func validateSourceURL(source, label, raw string) error {
	value, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (value.Scheme != "http" && value.Scheme != "https") || value.Host == "" || value.User != nil || value.RawQuery != "" || value.Fragment != "" {
		return fmt.Errorf("source %s %s must be an absolute HTTP(S) URL without credentials, query, or fragment", source, label)
	}
	return nil
}

func cookieHeader(fields map[string]string, names ...string) (string, error) {
	parts := make([]string, 0, len(names))
	for _, name := range names {
		value, exists := fields[name]
		if !exists {
			continue
		}
		for _, ch := range value {
			if ch <= 0x20 || ch >= 0x7f || strings.ContainsRune(";\"'", ch) {
				return "", errors.New("invalid cookie configuration")
			}
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; "), nil
}
