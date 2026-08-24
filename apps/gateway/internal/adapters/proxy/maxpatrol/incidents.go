package maxpatrol

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const incidentReadModelPath = "api/incident_read_model/v1/incidents"

func (client *Client) SearchIncidents(ctx context.Context, access Access, request IncidentSearchRequest) (IncidentPage, error) {
	if err := request.TimeRange.validate(); err != nil {
		return IncidentPage{}, err
	}
	if request.Limit < 1 || request.Limit > defaultChildPageSize {
		return IncidentPage{}, &RequestError{Operation: "incident search", Message: fmt.Sprintf("limit must be between 1 and %d", defaultChildPageSize)}
	}
	if request.Offset < 0 {
		return IncidentPage{}, &RequestError{Operation: "incident search", Message: "offset must not be negative"}
	}
	payload := incidentListPayload{
		Filter: incidentListFilter{
			DetectedAt: incidentTimeFilter{
				From: vendorTime(request.TimeRange.From),
				To:   vendorTime(request.TimeRange.To),
			},
			IsRemoved:  incidentBooleanFilter{Value: "false"},
			IsArchived: incidentBooleanFilter{Value: "false"},
		},
		Sorting: []struct{}{},
	}
	query := url.Values{
		"limit":  []string{intQuery(request.Limit)},
		"offset": []string{intQuery(request.Offset)},
	}
	var response incidentListEnvelope
	if err := client.doJSON(ctx, client.incidents, access, "incident search", http.MethodPost, incidentReadModelPath, query, payload, &response); err != nil {
		return IncidentPage{}, err
	}
	if response.TotalItems < 0 || request.Offset+len(response.Incidents) > response.TotalItems {
		return IncidentPage{}, &ResponseError{Operation: "incident search", Message: "pagination metadata is inconsistent"}
	}

	incidents := make([]Incident, 0, len(response.Incidents))
	seen := make(map[string]struct{}, len(response.Incidents))
	for index := range response.Incidents {
		incident, err := sanitizeIncident(response.Incidents[index])
		if err != nil {
			return IncidentPage{}, &ResponseError{Operation: "incident search", Message: fmt.Sprintf("incident %d is invalid", index)}
		}
		if _, exists := seen[incident.ID]; exists {
			continue
		}
		seen[incident.ID] = struct{}{}
		incidents = append(incidents, incident)
	}
	page := IncidentPage{
		Incidents:  incidents,
		TotalItems: response.TotalItems,
		Offset:     request.Offset,
	}
	consumed := request.Offset + len(response.Incidents)
	if consumed < response.TotalItems {
		next := consumed
		page.NextOffset = &next
	}
	return page, nil
}

func (client *Client) GetIncident(ctx context.Context, access Access, externalID string) (Incident, error) {
	id, err := validateUUID(externalID)
	if err != nil {
		return Incident{}, err
	}
	var incident Incident
	if err := client.doJSON(ctx, client.incidents, access, "incident detail", http.MethodGet, incidentReadModelPath+"/"+id, nil, nil, &incident); err != nil {
		return Incident{}, err
	}
	incident, err = sanitizeIncident(incident)
	if err != nil {
		return Incident{}, &ResponseError{Operation: "incident detail", Message: "incident is invalid"}
	}
	if incident.ID != id {
		return Incident{}, &ResponseError{Operation: "incident detail", Message: "incident identity does not match the request"}
	}
	return incident, nil
}

func (client *Client) ResolveIncident(ctx context.Context, access Access, request IncidentResolveRequest) (IncidentResolution, error) {
	if err := request.TimeRange.validate(); err != nil {
		return IncidentResolution{}, err
	}
	root, err := client.GetIncident(ctx, access, request.ExternalID)
	if err != nil {
		return IncidentResolution{}, err
	}
	result := IncidentResolution{Incident: root, Complete: true}

	events, truncated, err := fetchIncidentPages(client, ctx, access, root.ID, "events", func(page incidentChildrenEnvelope) []IncidentEvent { return page.Events })
	if err != nil {
		result.Errors = append(result.Errors, contextError("incident.events", err))
	}
	result.Truncated = result.Truncated || truncated

	accounts, accountsTruncated, err := fetchIncidentPages(client, ctx, access, root.ID, "accounts", func(page incidentChildrenEnvelope) []IncidentAccount { return page.Accounts })
	result.Accounts = dedupeAccounts(accounts)
	if err != nil {
		result.Errors = append(result.Errors, contextError("incident.accounts", err))
	}
	result.Truncated = result.Truncated || accountsTruncated

	files, filesTruncated, err := fetchIncidentPages(client, ctx, access, root.ID, "files", func(page incidentChildrenEnvelope) []IncidentFile { return page.Files })
	result.Files = sanitizeFiles(files)
	if err != nil {
		result.Errors = append(result.Errors, contextError("incident.files", err))
	}
	result.Truncated = result.Truncated || filesTruncated

	links, linksTruncated, err := fetchIncidentPages(client, ctx, access, root.ID, "external_source_links", func(page incidentChildrenEnvelope) []IncidentExternalLink { return page.Links })
	result.Links = client.dedupeLinks(links)
	if err != nil {
		result.Errors = append(result.Errors, contextError("incident.links", err))
	}
	result.Truncated = result.Truncated || linksTruncated

	assetGroups, groupsTruncated, err := fetchIncidentPages(client, ctx, access, root.ID, "asset_groups", func(page incidentChildrenEnvelope) []IncidentAssetGroup { return page.Items })
	result.AssetGroups = dedupeAssetGroups(assetGroups)
	if err != nil {
		result.Errors = append(result.Errors, contextError("incident.asset_groups", err))
	}
	result.Truncated = result.Truncated || groupsTruncated

	hosts, hostsTruncated, err := client.fetchIncidentHosts(ctx, access, root.ID)
	result.Hosts = dedupeHosts(hosts)
	if err != nil {
		result.Errors = append(result.Errors, contextError("incident.hosts", err))
	}
	result.Truncated = result.Truncated || hostsTruncated

	seenCorrelations := make(map[string]struct{}, len(events))
	seenEvents := make(map[string]struct{}, len(events))
	for index := range events {
		event := sanitizeIncidentEvent(events[index])
		fullInfo := event.FullInfo.record()
		if strings.TrimSpace(fullInfo.CorrelationName) == "" {
			raw, mappingErr := eventToRaw(fullInfo)
			if mappingErr != nil {
				result.Errors = append(result.Errors, ContextError{Component: "incident.events", Code: "invalid_response", Message: "an incident event is invalid"})
				continue
			}
			if _, exists := seenEvents[raw.UUID]; !exists {
				seenEvents[raw.UUID] = struct{}{}
				result.Events = append(result.Events, raw)
			}
			continue
		}
		correlation, mappingErr := eventToCorrelation(fullInfo)
		if mappingErr != nil {
			result.Errors = append(result.Errors, ContextError{Component: "incident.correlations", Code: "invalid_response", Message: "an incident correlation is invalid"})
			continue
		}
		if _, exists := seenCorrelations[correlation.UUID]; exists {
			continue
		}
		seenCorrelations[correlation.UUID] = struct{}{}
		resolution := client.resolveCorrelationRecord(ctx, access, request.TimeRange, correlation)
		result.Correlations = append(result.Correlations, resolution)
		if !resolution.Complete {
			result.Errors = append(result.Errors, ContextError{
				Component: "incident.correlations", Code: "partial", Message: "a child correlation has partial context",
				Retryable: retryableContextFailure(resolution.Errors) != nil,
			})
		}
	}

	result.Complete = len(result.Errors) == 0 && !result.Truncated
	sort.Slice(result.Correlations, func(left, right int) bool {
		return result.Correlations[left].Correlation.UUID < result.Correlations[right].Correlation.UUID
	})
	sort.Slice(result.Events, func(left, right int) bool { return result.Events[left].UUID < result.Events[right].UUID })
	return result, nil
}

func fetchIncidentPages[T any](
	client *Client,
	ctx context.Context,
	access Access,
	incidentID string,
	component string,
	selectItems func(incidentChildrenEnvelope) []T,
) ([]T, bool, error) {
	items := make([]T, 0)
	offset := 0
	total := -1
	fetched := 0
	for {
		if fetched >= client.maxChildItems {
			return items, true, nil
		}
		limit := client.childPageSize
		if remaining := client.maxChildItems - fetched; limit > remaining {
			limit = remaining
		}
		query := url.Values{
			"limit":  []string{intQuery(limit)},
			"offset": []string{intQuery(offset)},
		}
		var page incidentChildrenEnvelope
		operation := "incident " + component
		requestPath := incidentReadModelPath + "/" + incidentID + "/" + component
		if err := client.doJSON(ctx, client.incidents, access, operation, http.MethodGet, requestPath, query, nil, &page); err != nil {
			return items, false, err
		}
		pageItems := selectItems(page)
		if page.TotalItems < 0 || offset+len(pageItems) > page.TotalItems {
			return items, false, &ResponseError{Operation: operation, Message: "pagination metadata is inconsistent"}
		}
		if total == -1 {
			total = page.TotalItems
		} else if page.TotalItems != total {
			return items, false, &ResponseError{Operation: operation, Message: "totalItems changed during pagination"}
		}
		items = append(items, pageItems...)
		fetched += len(pageItems)
		offset += len(pageItems)
		if offset >= total {
			return items, false, nil
		}
		if len(pageItems) == 0 {
			return items, false, &ResponseError{Operation: operation, Message: "pagination did not advance"}
		}
	}
}

func (client *Client) fetchIncidentHosts(ctx context.Context, access Access, incidentID string) ([]IncidentHost, bool, error) {
	var hosts []IncidentHost
	requestPath := incidentReadModelPath + "/" + incidentID + "/hosts"
	if err := client.doJSON(ctx, client.incidents, access, "incident hosts", http.MethodGet, requestPath, nil, nil, &hosts); err != nil {
		return nil, false, err
	}
	if len(hosts) > client.maxChildItems {
		return hosts[:client.maxChildItems], true, nil
	}
	return hosts, false, nil
}

func sanitizeIncident(value Incident) (Incident, error) {
	id, err := validateUUID(value.ID)
	if err != nil || value.DetectedAt.IsZero() {
		return Incident{}, fmt.Errorf("invalid incident identity or detectedAt")
	}
	value.ID = id
	value.Key = cleanIdentifier(value.Key)
	value.ExternalKey = cleanIdentifier(value.ExternalKey)
	value.ExternalID = cleanIdentifier(value.ExternalID)
	value.Name = cleanText(value.Name, maxNameLength)
	value.Source = IncidentSource{
		Type: cleanText(value.Source.Type, maxNameLength),
		ID:   cleanIdentifier(value.Source.ID),
		Name: cleanText(value.Source.Name, maxNameLength),
	}
	value.Severity = normalizedSeverity(value.Severity)
	value.DetectedAt = value.DetectedAt.UTC()
	value.Verdict = cleanText(value.Verdict, maxNameLength)
	value.Description = cleanText(value.Description, maxDescriptionLength)
	value.Recommendation = cleanText(value.Recommendation, maxDescriptionLength)
	value.Damage = cleanText(value.Damage, maxNameLength)
	value.Type = cleanText(value.Type, maxNameLength)
	value.State = cleanText(value.State, maxNameLength)
	value.CreatedAt = value.CreatedAt.UTC()
	value.ChangedAt = value.ChangedAt.UTC()
	if value.AssignedTo != nil {
		value.AssignedTo.ID = cleanIdentifier(value.AssignedTo.ID)
		value.AssignedTo.Name = cleanText(value.AssignedTo.Name, maxNameLength)
	}
	if value.ParentID != nil {
		cleaned := cleanIdentifier(*value.ParentID)
		value.ParentID = &cleaned
	}
	return value, nil
}

func sanitizeIncidentEvent(value IncidentEvent) IncidentEvent {
	value.ID = cleanIdentifier(value.ID)
	value.ExternalID = cleanIdentifier(value.ExternalID)
	value.EventKey = cleanText(value.EventKey, maxNameLength)
	value.Description = cleanText(value.Description, maxDescriptionLength)
	value.Type = cleanText(value.Type, maxNameLength)
	value.DetectedAt = value.DetectedAt.UTC()
	return value
}

func dedupeAccounts(values []IncidentAccount) []IncidentAccount {
	result := make([]IncidentAccount, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.ID = cleanIdentifier(value.ID)
		value.Name = strings.ToLower(cleanText(value.Name, maxNameLength))
		key := value.ID
		if key == "" {
			key = value.Name
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Name != result[right].Name {
			return result[left].Name < result[right].Name
		}
		return result[left].ID < result[right].ID
	})
	return result
}

func dedupeHosts(values []IncidentHost) []IncidentHost {
	result := make([]IncidentHost, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.ID = cleanIdentifier(value.ID)
		value.FQDN = normalizeHost(value.FQDN)
		value.Role = cleanText(value.Role, maxNameLength)
		if value.IP != nil {
			ip := normalizeIP(*value.IP)
			if ip == "" {
				value.IP = nil
			} else {
				value.IP = &ip
			}
		}
		if value.AssetID != nil {
			assetID := cleanIdentifier(*value.AssetID)
			value.AssetID = &assetID
		}
		key := value.ID
		if key == "" {
			key = value.FQDN + "\x00" + value.Role
			if value.IP != nil {
				key += "\x00" + *value.IP
			}
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].FQDN != result[right].FQDN {
			return result[left].FQDN < result[right].FQDN
		}
		return result[left].ID < result[right].ID
	})
	return result
}

func sanitizeFiles(values []IncidentFile) []IncidentFile {
	result := make([]IncidentFile, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.ID = cleanIdentifier(value.ID)
		value.Name = cleanText(value.Name, maxNameLength)
		value.Path = cleanText(value.Path, maxAttributeLength)
		value.MD5 = strings.ToLower(cleanIdentifier(value.MD5))
		value.SHA1 = strings.ToLower(cleanIdentifier(value.SHA1))
		value.SHA256 = strings.ToLower(cleanIdentifier(value.SHA256))
		if value.Size < 0 {
			value.Size = 0
		}
		key := firstNonEmpty(value.ID, value.SHA256, value.SHA1, value.MD5, value.Path+"\x00"+value.Name)
		if key == "\x00" || key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (client *Client) dedupeLinks(values []IncidentExternalLink) []IncidentExternalLink {
	result := make([]IncidentExternalLink, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = client.sanitizeLink(value)
		key := value.Name + "\x00" + value.URL
		if key == "\x00" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func dedupeAssetGroups(values []IncidentAssetGroup) []IncidentAssetGroup {
	result := make([]IncidentAssetGroup, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.ID = cleanIdentifier(value.ID)
		value.Name = cleanText(value.Name, maxNameLength)
		value.Description = cleanText(value.Description, maxDescriptionLength)
		key := firstNonEmpty(value.ID, value.Name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
