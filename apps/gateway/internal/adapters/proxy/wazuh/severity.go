package wazuh

import (
	"path"
	"strings"
	"time"
)

func mapSeverity(level int) string {
	switch {
	case level >= 0 && level <= 2:
		return "info"
	case level >= 3 && level <= 5:
		return "low"
	case level >= 6 && level <= 9:
		return "medium"
	case level >= 10 && level <= 12:
		return "high"
	case level >= 13 && level <= 16:
		return "critical"
	default:
		return "unknown"
	}
}

func mapEventType(groups []string) string {
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[strings.ToLower(strings.TrimSpace(group))] = struct{}{}
	}
	has := func(name string) bool {
		_, ok := set[name]
		return ok
	}
	switch {
	case has("authentication_success"):
		return "authentication.success"
	case has("authentication_failed"), has("authentication_failures"), has("win_authentication_failed"), has("invalid_login"):
		return "authentication.failure"
	case has("syscheck"):
		return "file.integrity"
	case has("vulnerability-detector"):
		return "vulnerability.detection"
	case has("virustotal"), has("yara"), has("rootcheck"):
		return "malware.detection"
	case has("web_scan"), has("recon"), has("attack"), has("ids"), has("firewall"), has("aws_guardduty"):
		return "network.alert"
	case has("sudo"), has("audit_command"), has("pam"):
		return "process.audit"
	case has("sca"), has("ciscat"), has("oscap"), has("policy_changed"):
		return "compliance.check"
	case has("office365"), has("ms-graph"), has("github"), has("gcp"), has("aws"), has("docker"), has("osquery"):
		return "cloud.audit"
	default:
		return "security.alert"
	}
}

func hasGroup(groups []string, wanted string) bool {
	wanted = strings.ToLower(wanted)
	for _, group := range groups {
		if strings.ToLower(strings.TrimSpace(group)) == wanted {
			return true
		}
	}
	return false
}

func parseOccurredAt(source alertSource) (time.Time, error) {
	candidates := []string{source.AtTimestamp, source.Timestamp}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05.000+0000",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, candidate); err == nil {
				return parsed.UTC(), nil
			}
		}
		if parsed, err := time.Parse("2006-01-02T15:04:05.999999999Z07:00", candidate); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, &ResponseError{Operation: "event search", Message: "alert timestamp is missing"}
}

func boundText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func basename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return boundText(path.Base(strings.ReplaceAll(value, "\\", "/")), 128)
}

func putAttr(attrs map[string]any, key, value string) {
	value = boundText(value, 256)
	if value == "" {
		return
	}
	attrs[key] = value
}

func putAttrInt(attrs map[string]any, key string, value int) {
	if value == 0 {
		return
	}
	attrs[key] = value
}

func putAttrList(attrs map[string]any, key string, values []string) {
	cleaned := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = boundText(value, 128)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
		if len(cleaned) >= 32 {
			break
		}
	}
	if len(cleaned) == 0 {
		return
	}
	attrs[key] = cleaned
}
