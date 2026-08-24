package maxpatrol

import (
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

const (
	maxIdentifierLength  = 256
	maxNameLength        = 1024
	maxDescriptionLength = 8192
	maxAttributeLength   = 4096
)

var (
	sensitiveAssignment = regexp.MustCompile(`(?i)\b(password|passwd|pwd|cookie|authorization|ntlm(?:_response|response)?|access[_-]?token|refresh[_-]?token)\b\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`)
	sensitiveArgument   = regexp.MustCompile(`(?i)(--?(?:password|passwd|pwd|token)\s+)("[^"]*"|'[^']*'|\S+)`)
	urlUserinfo         = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)
)

func cleanIdentifier(value string) string {
	return cleanText(value, maxIdentifierLength)
}

func cleanText(value string, limit int) string {
	value = strings.Map(func(character rune) rune {
		switch character {
		case '\r', '\n', '\t':
			return ' '
		}
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	// Assignment values such as "Authorization: Bearer ..." and Cookie can
	// contain whitespace or delimiter-separated material. Redacting only the
	// first token can expose the remainder, so discard the whole bounded field
	// whenever a sensitive assignment is present.
	if sensitiveAssignment.MatchString(value) {
		value = "<redacted>"
	} else {
		value = sensitiveArgument.ReplaceAllString(value, "$1<redacted>")
		value = urlUserinfo.ReplaceAllString(value, "$1<redacted>@")
	}
	characters := []rune(value)
	if len(characters) > limit {
		value = string(characters[:limit])
	}
	return strings.TrimSpace(value)
}

func validateUUID(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != strings.ToLower(value) {
		return "", &RequestError{Operation: "record identity", Message: "external_id must be a canonical UUID"}
	}
	return parsed.String(), nil
}

func normalizedSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "info", "low", "medium", "high", "critical":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func normalizeHost(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.TrimSuffix(value, ".")
}

func normalizeIP(value string) string {
	parsed := net.ParseIP(strings.TrimSpace(value))
	if parsed == nil {
		return ""
	}
	return parsed.String()
}

func normalizeMAC(value string) string {
	parsed, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.String())
}

func normalizeAccount(name, domain string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	domain = strings.ToLower(strings.TrimSpace(domain))
	if name == "" {
		return ""
	}
	if domain == "" || strings.ContainsAny(name, "\\@") {
		return name
	}
	return domain + "\\" + name
}

func entityMentions(record safeEventRecord) []EntityMention {
	mentions := make([]EntityMention, 0, 16)
	seen := make(map[string]struct{}, 16)
	add := func(kind, value, role string) {
		switch kind {
		case "ip":
			value = normalizeIP(value)
		case "mac":
			value = normalizeMAC(value)
		case "host":
			value = normalizeHost(value)
		case "account":
			value = strings.ToLower(strings.TrimSpace(value))
		}
		if value == "" {
			return
		}
		key := kind + "\x00" + value + "\x00" + role
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		mentions = append(mentions, EntityMention{Type: kind, Value: value, Role: role})
	}

	for _, host := range []string{record.EventSourceHost, record.EventSourceFQDN, record.EventSourceHostname} {
		add("host", host, "mentions")
	}
	add("ip", record.EventSourceIP, "mentions")
	add("mac", record.EventSourceMAC, "mentions")
	for _, host := range []string{record.SourceHost, record.SourceHostname} {
		add("host", host, "src")
	}
	add("ip", record.SourceIP, "src")
	add("mac", record.SourceMAC, "src")
	for _, host := range []string{record.DestinationHost, record.DestinationHostname, record.DestinationFQDN} {
		add("host", host, "dst")
	}
	add("ip", record.DestinationIP, "dst")
	add("mac", record.DestinationMAC, "dst")
	for _, host := range []string{record.ExternalDestinationHost, record.ExternalDestinationHostname, record.ExternalDestinationFQDN} {
		add("host", host, "dst")
	}
	add("account", normalizeAccount(record.SubjectAccountName, record.SubjectAccountDomain), "actor")
	add("account", normalizeAccount(record.ObjectAccountName, record.ObjectAccountDomain), "object")

	sort.Slice(mentions, func(left, right int) bool {
		if mentions[left].Type != mentions[right].Type {
			return mentions[left].Type < mentions[right].Type
		}
		if mentions[left].Value != mentions[right].Value {
			return mentions[left].Value < mentions[right].Value
		}
		return mentions[left].Role < mentions[right].Role
	})
	return mentions
}

func eventToCorrelation(record safeEventRecord) (Correlation, error) {
	id, err := validateUUID(record.UUID)
	if err != nil {
		return Correlation{}, &ResponseError{Operation: "correlation event", Message: "uuid is invalid"}
	}
	if record.Time.IsZero() {
		return Correlation{}, &ResponseError{Operation: "correlation event", Message: "time is missing"}
	}
	ruleName := cleanText(record.CorrelationName, maxNameLength)
	if ruleName == "" {
		return Correlation{}, &ResponseError{Operation: "correlation event", Message: "correlation_name is missing"}
	}
	subeventIDs := make([]string, 0, len(record.SubeventIDs))
	seen := make(map[string]struct{}, len(record.SubeventIDs))
	for _, candidate := range record.SubeventIDs {
		candidate, validationErr := validateUUID(candidate)
		if validationErr != nil {
			return Correlation{}, &ResponseError{Operation: "correlation event", Message: "subevent identity is invalid"}
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		subeventIDs = append(subeventIDs, candidate)
	}
	count := record.SubeventCount
	if count < len(subeventIDs) {
		count = len(subeventIDs)
	}
	return Correlation{
		UUID:            id,
		RuleName:        ruleName,
		CorrelationType: cleanText(record.CorrelationType, maxNameLength),
		Title:           cleanText(record.Text, maxDescriptionLength),
		Severity:        normalizedSeverity(record.Importance),
		OccurredAt:      record.Time.UTC(),
		Action:          cleanText(record.Action, maxNameLength),
		SubeventCount:   count,
		SubeventIDs:     subeventIDs,
		Entities:        entityMentions(record),
	}, nil
}

func eventToRaw(record safeEventRecord) (RawEvent, error) {
	id, err := validateUUID(record.UUID)
	if err != nil {
		return RawEvent{}, &ResponseError{Operation: "raw event", Message: "uuid is invalid"}
	}
	if record.Time.IsZero() {
		return RawEvent{}, &ResponseError{Operation: "raw event", Message: "time is missing"}
	}
	return RawEvent{
		UUID:            id,
		Title:           cleanText(record.Text, maxDescriptionLength),
		Severity:        normalizedSeverity(record.Importance),
		OccurredAt:      record.Time.UTC(),
		Action:          cleanText(record.Action, maxNameLength),
		EventSourceHost: normalizeHost(firstNonEmpty(record.EventSourceFQDN, record.EventSourceHost, record.EventSourceHostname)),
		EventSourceIP:   normalizeIP(record.EventSourceIP),
		SourceHost:      normalizeHost(firstNonEmpty(record.SourceHost, record.SourceHostname)),
		SourceIP:        normalizeIP(record.SourceIP),
		SourcePort:      validPort(record.SourcePort),
		DestinationHost: normalizeHost(firstNonEmpty(record.DestinationFQDN, record.DestinationHost, record.DestinationHostname, record.ExternalDestinationFQDN, record.ExternalDestinationHost, record.ExternalDestinationHostname)),
		DestinationIP:   normalizeIP(record.DestinationIP),
		DestinationPort: validPort(record.DestinationPort),
		ActorAccount:    normalizeAccount(record.SubjectAccountName, record.SubjectAccountDomain),
		ObjectAccount:   normalizeAccount(record.ObjectAccountName, record.ObjectAccountDomain),
		SubjectProcess: ProcessHint{
			Name:        cleanText(record.SubjectProcessName, maxNameLength),
			Path:        cleanText(record.SubjectProcessPath, maxAttributeLength),
			CommandLine: cleanText(record.SubjectProcessCommand, maxAttributeLength),
		},
		ObjectProcess: ProcessHint{
			Name:        cleanText(record.ObjectProcessName, maxNameLength),
			Path:        cleanText(record.ObjectProcessPath, maxAttributeLength),
			CommandLine: cleanText(record.ObjectProcessCommand, maxAttributeLength),
		},
		ObjectName:      cleanText(record.ObjectName, maxNameLength),
		ObjectPath:      cleanText(record.ObjectPath, maxAttributeLength),
		CategoryGeneric: cleanText(record.CategoryGeneric, maxNameLength),
		CategoryHigh:    cleanText(record.CategoryHigh, maxNameLength),
		CategoryLow:     cleanText(record.CategoryLow, maxNameLength),
		Entities:        entityMentions(record),
	}, nil
}

func validPort(value int64) int64 {
	if value < 1 || value > 65535 {
		return 0
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (client *Client) sanitizeLink(value IncidentExternalLink) IncidentExternalLink {
	result := IncidentExternalLink{Name: cleanText(value.Name, maxNameLength)}
	parsed, err := url.Parse(strings.TrimSpace(value.URL))
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return result
	}
	if _, internal := client.internalHosts[strings.ToLower(parsed.Host)]; internal {
		result.Internal = true
		return result
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	result.URL = parsed.String()
	return result
}
