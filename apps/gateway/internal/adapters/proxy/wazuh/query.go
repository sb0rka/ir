package wazuh

import (
	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/fieldmap"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

func buildQuery(from, to time.Time, entities []domain.EntityRef, filter string, groupBy []string, groupValues []*string) (map[string]any, error) {
	if from.IsZero() || !from.Before(to) {
		return nil, sourceRequestError("invalid_time_range", "time_from must be earlier than time_to")
	}
	filters := []any{map[string]any{"range": map[string]any{"timestamp": map[string]any{"gte": from.UTC().Format(time.RFC3339Nano), "lte": to.UTC().Format(time.RFC3339Nano)}}}}
	eq, _, err := entityFilterQuery(entities)
	if err != nil {
		return nil, err
	}
	if eq != nil {
		filters = append(filters, eq)
	}
	fq, err := filterToQuery(filter)
	if err != nil {
		return nil, err
	}
	if fq != nil {
		filters = append(filters, fq)
	}
	groups, err := groupValueFilters(groupBy, groupValues)
	if err != nil {
		return nil, err
	}
	filters = append(filters, groups...)
	return map[string]any{"bool": map[string]any{"filter": filters}}, nil
}
func (client *Client) buildSort(rules []capability.EventSort) ([]map[string]any, error) {
	clauses := []map[string]any{}
	seen := map[string]bool{}
	for _, rule := range rules {
		spec, ok := lookupField(rule.Field)
		if !ok || (rule.Direction != "asc" && rule.Direction != "desc") {
			return nil, sourceRequestError("invalid_source_request", "sort contains an unsupported field or direction")
		}
		if seen[spec.Path] {
			return nil, sourceRequestError("invalid_source_request", "sort fields must be unique")
		}
		seen[spec.Path] = true
		clauses = append(clauses, map[string]any{spec.Path: map[string]any{"order": rule.Direction, "missing": "_last", "unmapped_type": string(spec.Type)}})
	}
	for _, tie := range []struct{ path, direction string }{{"timestamp", "desc"}, {"_index", "asc"}, {client.documentIDField, "asc"}} {
		if !seen[tie.path] {
			clauses = append(clauses, map[string]any{tie.path: map[string]any{"order": tie.direction}})
			seen[tie.path] = true
		}
	}
	return clauses, nil
}
func entityFilterQuery(entities []domain.EntityRef) (map[string]any, []string, error) {
	if len(entities) == 0 {
		return nil, nil, nil
	}
	should := make([]any, 0, len(entities)*4)
	keys := make([]string, 0, len(entities))
	for _, entity := range entities {
		kind := strings.ToLower(strings.TrimSpace(entity.Type))
		value := domain.CanonicalValue(kind, entity.Value)
		if len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
			return nil, nil, &RequestError{Operation: "event search", Message: "entity value is invalid"}
		}
		keys = append(keys, kind+":"+value)
		fields, err := entityFields(kind, value)
		if err != nil {
			return nil, nil, err
		}
		for _, field := range fields {
			clause := map[string]any{"value": value}
			if field.CaseInsensitive {
				clause["case_insensitive"] = true
			}
			should = append(should, map[string]any{"term": map[string]any{field.Path: clause}})
		}
	}
	sort.Strings(keys)
	return map[string]any{"bool": map[string]any{"should": should, "minimum_should_match": 1}}, keys, nil
}

type entityField struct {
	Path            string
	CaseInsensitive bool
}

func entityFields(kind, value string) ([]entityField, error) {
	switch kind {
	case "ip":
		if net.ParseIP(value) == nil {
			return nil, &RequestError{Operation: "event search", Message: "IP entity value is invalid"}
		}
		paths, err := fieldmap.Paths(SourceCode, "ip")
		if err != nil {
			return nil, err
		}
		result := []entityField{}
		for _, p := range paths {
			result = append(result, entityField{Path: p.Path, CaseInsensitive: p.CaseInsensitive})
		}
		return result, nil
	case "host", "hostname":
		paths, err := fieldmap.Paths(SourceCode, "host")
		if err != nil {
			return nil, err
		}
		result := []entityField{}
		for _, p := range paths {
			result = append(result, entityField{Path: p.Path, CaseInsensitive: p.CaseInsensitive})
		}
		return result, nil
	case "account", "user":
		return []entityField{
			{Path: "data.srcuser", CaseInsensitive: true},
			{Path: "data.dstuser", CaseInsensitive: true},
			{Path: "data.win.eventdata.targetUserName", CaseInsensitive: true},
			{Path: "data.office365.UserId", CaseInsensitive: true},
			{Path: "data.github.actor", CaseInsensitive: true},
			{Path: "data.github.user", CaseInsensitive: true},
		}, nil
	case "file_hash", "hash", "md5", "sha1", "sha256":
		return []entityField{
			{Path: "syscheck.md5_after"},
			{Path: "syscheck.sha1_after"},
			{Path: "syscheck.sha256_after"},
			{Path: "data.virustotal.source.md5"},
			{Path: "data.virustotal.source.sha1"},
		}, nil
	case "url":
		return []entityField{{Path: "data.url"}}, nil
	case "domain":
		return []entityField{{Path: "data.gcp.jsonPayload.queryName"}}, nil
	default:
		return nil, &RequestError{Operation: "event search", Message: "unsupported_entity_type"}
	}
}

func groupValueFilters(groupBy []string, groupValues []*string) ([]any, error) {
	if len(groupBy) != len(groupValues) {
		return nil, &RequestError{Operation: "event search", Message: "group_values must align with group_by"}
	}
	filters := make([]any, 0, len(groupBy))
	for index, field := range groupBy {
		spec, ok := lookupField(field)
		if !ok || spec.Path == "" {
			return nil, &RequestError{Operation: "event search", Message: "group_by contains an unsupported field"}
		}
		if groupValues[index] == nil {
			filters = append(filters, map[string]any{
				"bool": map[string]any{"must_not": []any{map[string]any{"exists": map[string]any{"field": spec.Path}}}},
			})
			continue
		}
		value := *groupValues[index]
		if len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") || !validFieldLiteral(spec, value) {
			return nil, &RequestError{Operation: "event search", Message: "group_values contains an invalid value"}
		}
		clause := map[string]any{"value": value}
		if spec.CaseInsensitive {
			clause["case_insensitive"] = true
		}
		filters = append(filters, map[string]any{"term": map[string]any{spec.Path: clause}})
	}
	return filters, nil
}

// PIT fixes the documents, but relative date math would still change the query
// between pages. Accept only values independent of the vendor's current clock.
func validFieldLiteral(spec fieldSpec, value string) bool {
	if spec.Type != fieldLong && spec.Type != fieldDate {
		return true
	}
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return true
	}
	if spec.Type == fieldDate {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700", time.DateOnly} {
			if _, err := time.Parse(layout, value); err == nil {
				return true
			}
		}
	}
	return false
}
