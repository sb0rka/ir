package wazuh

import (
	"net"
	"sort"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type searchBuild struct {
	Body        searchRequest
	Fingerprint string
	SortKeys    []string
}

func buildSearchRequest(client *Client, request capability.SearchEventsRequest) (searchBuild, error) {
	if request.TimeFrom.IsZero() || request.TimeTo.IsZero() || !request.TimeFrom.Before(request.TimeTo) {
		return searchBuild{}, &RequestError{Operation: "event search", Message: "time_from must be earlier than time_to"}
	}
	if request.Limit < 1 || request.Limit > 1000 {
		return searchBuild{}, &RequestError{Operation: "event search", Message: "limit must be between 1 and 1000"}
	}
	filters := make([]any, 0, 8)
	filters = append(filters, map[string]any{
		"range": map[string]any{
			"@timestamp": map[string]any{
				"gte": request.TimeFrom.UTC().Format(time.RFC3339Nano),
				"lte": request.TimeTo.UTC().Format(time.RFC3339Nano),
			},
		},
	})
	entityQuery, entityKeys, err := entityFilterQuery(request.Entities)
	if err != nil {
		return searchBuild{}, err
	}
	if entityQuery != nil {
		filters = append(filters, entityQuery)
	}
	filterQuery, err := filterToQuery(request.Filter)
	if err != nil {
		return searchBuild{}, err
	}
	if filterQuery != nil {
		filters = append(filters, filterQuery)
	}
	groupFilters, err := groupValueFilters(request.GroupBy, request.GroupValues)
	if err != nil {
		return searchBuild{}, err
	}
	filters = append(filters, groupFilters...)

	sortClauses, sortKeys, err := buildSort(request.Sort)
	if err != nil {
		return searchBuild{}, err
	}
	includes := append([]string(nil), sourceIncludes...)
	for _, column := range request.Columns {
		spec, ok := lookupField(column)
		if !ok || spec.Path == "" || spec.CorrelationType || spec.DocumentID {
			return searchBuild{}, &RequestError{Operation: "event search", Message: "columns contains an unsupported field"}
		}
		includes = append(includes, spec.Path)
	}

	fingerprint := cursorFingerprint(
		client.indexPattern,
		request.TimeFrom.UTC().Format(time.RFC3339Nano),
		request.TimeTo.UTC().Format(time.RFC3339Nano),
		entityKeys,
		request.Filter,
		sortKeys,
		request.GroupBy,
		request.GroupValues,
	)
	cursor, err := decodeCursor(request.Cursor, fingerprint)
	if err != nil {
		return searchBuild{}, err
	}

	body := searchRequest{
		Size:           request.Limit,
		TrackTotalHits: true,
		Timeout:        searchTimeout,
		Query:          map[string]any{"bool": map[string]any{"filter": filters}},
		Sort:           sortClauses,
		Source:         sourceFilter{Includes: includes},
	}
	if len(cursor.Sort) > 0 {
		body.SearchAfter = cursor.Sort
	}
	return searchBuild{Body: body, Fingerprint: fingerprint, SortKeys: sortKeys}, nil
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
		if value == "" || len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
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
		return []entityField{
			{Path: "agent.ip"},
			{Path: "data.srcip"},
			{Path: "data.dstip"},
			{Path: "data.win.eventdata.ipAddress"},
			{Path: "data.office365.ClientIP"},
			{Path: "data.gcp.jsonPayload.sourceIP"},
			{Path: "data.aws.service.action.networkConnectionAction.remoteIpDetails.ipAddressV4"},
			{Path: "data.aws.service.action.awsApiCallAction.remoteIpDetails.ipAddressV4"},
			{Path: "data.ms-graph.ipAddress"},
		}, nil
	case "host", "hostname":
		return []entityField{
			{Path: "agent.name", CaseInsensitive: true},
			{Path: "data.win.system.computer", CaseInsensitive: true},
			{Path: "data.system_name", CaseInsensitive: true},
			{Path: "data.gcp.jsonPayload.vmInstanceName", CaseInsensitive: true},
		}, nil
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
		if !ok || spec.Path == "" || spec.CorrelationType || spec.DocumentID {
			return nil, &RequestError{Operation: "event search", Message: "group_by contains an unsupported field"}
		}
		if groupValues[index] == nil {
			filters = append(filters, map[string]any{
				"bool": map[string]any{"must_not": []any{map[string]any{"exists": map[string]any{"field": spec.Path}}}},
			})
			continue
		}
		value := strings.TrimSpace(*groupValues[index])
		if value == "" || len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") {
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

func buildSort(rules []capability.EventSort) ([]map[string]any, []string, error) {
	if len(rules) == 0 {
		rules = []capability.EventSort{{Field: "time", Direction: "desc"}}
	}
	clauses := make([]map[string]any, 0, len(rules)+4)
	keys := make([]string, 0, len(rules))
	seen := map[string]struct{}{}
	for _, rule := range rules {
		field := strings.TrimSpace(rule.Field)
		direction := strings.ToLower(strings.TrimSpace(rule.Direction))
		if direction != "asc" && direction != "desc" {
			return nil, nil, &RequestError{Operation: "event search", Message: "sort direction must be asc or desc"}
		}
		path, err := sortPath(field)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		clauses = append(clauses, map[string]any{path: map[string]any{"order": direction}})
		keys = append(keys, field+":"+direction)
	}
	for _, tie := range []struct {
		path string
		dir  string
	}{
		{"@timestamp", "desc"},
		{"id", "asc"},
		{"_index", "asc"},
		{"_id", "asc"},
	} {
		if _, exists := seen[tie.path]; exists {
			continue
		}
		seen[tie.path] = struct{}{}
		clauses = append(clauses, map[string]any{tie.path: map[string]any{"order": tie.dir}})
	}
	return clauses, keys, nil
}

func sortPath(field string) (string, error) {
	switch field {
	case "time":
		return "@timestamp", nil
	case "uuid":
		return "_id", nil
	default:
		spec, ok := lookupField(field)
		if !ok || spec.Path == "" || spec.CorrelationType {
			return "", &RequestError{Operation: "event search", Message: "sort contains an unsupported field"}
		}
		return spec.Path, nil
	}
}
