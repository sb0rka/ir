package maxpatrol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

const (
	CredentialSecretName  = "DEMO_PT_SIEM_COOKIE"
	IncidentRecordType    = "siem_incident"
	CorrelationRecordType = "siem_correlation"
)

type Provider struct {
	client *Client
}

var (
	_ capability.FindingSource         = (*Provider)(nil)
	_ capability.EventSource           = (*Provider)(nil)
	_ capability.EntityLookup          = (*Provider)(nil)
	_ capability.AccountUserinfoSource = (*Provider)(nil)
	_ capability.SourceProber          = (*Provider)(nil)
)

// NewProvider builds the real provider and its two process-owned transports.
// Project credentials are supplied later through capability.Access.
func NewProvider(config ClientConfig) (registry.Provider, error) {
	client, err := NewClient(config)
	if err != nil {
		return registry.Provider{}, err
	}
	adapter := &Provider{client: client}
	return registry.Provider{
		Source: domain.Source{
			Code:   SourceCode,
			Name:   "MaxPatrol SIEM",
			Kind:   "siem",
			Mode:   "proxy",
			Status: "offline",
			Capabilities: []domain.Capability{
				domain.CapabilityFindings,
				domain.CapabilityEvents,
				domain.CapabilityEntityLookup,
				domain.CapabilityAccountUserinfo,
			},
		},
		CredentialSecret: CredentialSecretName,
		Findings:         adapter,
		Events:           adapter,
		EntityLookup:     adapter,
		AccountUserinfo:  adapter,
		Prober:           adapter,
	}, nil
}

type findingCursor struct {
	IncidentOffset   int  `json:"incident_offset"`
	IncidentDone     bool `json:"incident_done"`
	CorrelationsDone bool `json:"correlations_done"`
}

func (provider *Provider) SearchFindings(ctx context.Context, access capability.Access, request capability.SearchFindingsRequest) (capability.FindingPage, error) {
	if provider == nil || provider.client == nil {
		return capability.FindingPage{}, sourceRequestError("source_unavailable", "MaxPatrol client is not configured")
	}
	if err := validateDomainTimeRange(request.TimeRange); err != nil {
		return capability.FindingPage{}, err
	}
	if request.Limit < 1 || request.Limit > 1000 {
		return capability.FindingPage{}, sourceRequestError("invalid_limit", "limit must be between 1 and 1000")
	}
	wantIncidents, wantCorrelations := findingKinds(request.Kinds)
	cursor, err := decodeFindingCursor(request.Cursor)
	if err != nil {
		return capability.FindingPage{}, err
	}
	if !wantIncidents {
		cursor.IncidentDone = true
	}
	if !wantCorrelations {
		cursor.CorrelationsDone = true
	}

	page := capability.FindingPage{Status: "complete"}
	remaining := request.Limit
	fetchedAt := provider.client.now().UTC()
	vendorRange := TimeRange{From: request.TimeRange.From, To: request.TimeRange.To}
	accessValue := Access{Cookie: access.Cookie}

	if !cursor.IncidentDone && remaining > 0 {
		incidents, callErr := provider.client.SearchIncidents(ctx, accessValue, IncidentSearchRequest{
			TimeRange: vendorRange,
			Limit:     remaining,
			Offset:    cursor.IncidentOffset,
		})
		if callErr != nil {
			return capability.FindingPage{}, translateError(callErr)
		}
		for _, incident := range incidents.Incidents {
			page.Findings = append(page.Findings, findingFromIncident(incident, request.TimeRange, fetchedAt, nil))
		}
		remaining -= len(incidents.Incidents)
		if incidents.NextOffset != nil {
			cursor.IncidentOffset = *incidents.NextOffset
		} else {
			cursor.IncidentDone = true
		}
	}

	if cursor.IncidentDone && !cursor.CorrelationsDone && remaining > 0 {
		correlations, callErr := provider.client.SearchCorrelations(ctx, accessValue, CorrelationSearchRequest{
			TimeRange: vendorRange,
			Limit:     remaining,
		})
		if callErr != nil {
			return capability.FindingPage{}, translateError(callErr)
		}
		for _, correlation := range correlations.Correlations {
			page.Findings = append(page.Findings, findingFromCorrelation(correlation, request.TimeRange, fetchedAt))
		}
		cursor.CorrelationsDone = true
		if correlations.Truncated {
			page.Status = "truncated"
		}
	}

	if !cursor.IncidentDone || !cursor.CorrelationsDone {
		encoded, encodeErr := encodeFindingCursor(cursor)
		if encodeErr != nil {
			return capability.FindingPage{}, encodeErr
		}
		page.NextCursor = encoded
	}
	return page, nil
}

func (provider *Provider) ResolveFinding(ctx context.Context, access capability.Access, ref domain.SourceObjectRef) (capability.ContextPage, error) {
	if err := validateFindingRef(ref); err != nil {
		return capability.ContextPage{}, err
	}
	switch ref.RecordType {
	case IncidentRecordType:
		resolution, err := provider.client.ResolveIncident(ctx, Access{Cookie: access.Cookie}, IncidentResolveRequest{
			ExternalID: ref.ExternalID,
			TimeRange:  TimeRange{From: ref.TimeRange.From, To: ref.TimeRange.To},
		})
		if err != nil {
			return capability.ContextPage{}, translateError(err)
		}
		page := provider.incidentContext(ref.TimeRange, resolution)
		return page, retryableContextFailure(resolution.Errors)
	case CorrelationRecordType:
		resolution, err := provider.client.ResolveCorrelation(ctx, Access{Cookie: access.Cookie}, CorrelationResolveRequest{
			ExternalID: ref.ExternalID,
			TimeRange:  TimeRange{From: ref.TimeRange.From, To: ref.TimeRange.To},
		})
		if err != nil {
			return capability.ContextPage{}, translateError(err)
		}
		page := provider.correlationContext(ref.TimeRange, resolution)
		return page, retryableContextFailure(resolution.Errors)
	default:
		return capability.ContextPage{}, sourceRequestError("invalid_source_ref", "MaxPatrol finding record_type is not supported")
	}
}

func (provider *Provider) SearchEvents(ctx context.Context, access capability.Access, request capability.SearchEventsRequest) (capability.EventPage, error) {
	if request.TimeFrom.IsZero() || request.TimeTo.IsZero() || !request.TimeFrom.Before(request.TimeTo) {
		return capability.EventPage{}, sourceRequestError("invalid_time_range", "time_from must be earlier than time_to")
	}
	if request.Cursor != "" {
		return capability.EventPage{}, sourceRequestError("invalid_cursor", "MaxPatrol event continuation is not confirmed")
	}
	if request.Limit < 1 || request.Limit > 1000 {
		return capability.EventPage{}, sourceRequestError("invalid_limit", "limit must be between 1 and 1000")
	}
	where, err := eventWhere(request.Entities)
	if err != nil {
		return capability.EventPage{}, err
	}
	query, err := buildEventSearchQuery(request, where)
	if err != nil {
		return capability.EventPage{}, err
	}
	vendorRange := TimeRange{From: request.TimeFrom, To: request.TimeTo}
	response, err := provider.client.searchEventsWithQuery(ctx, Access{Cookie: access.Cookie}, vendorRange, query)
	if err != nil {
		return capability.EventPage{}, translateError(err)
	}
	fetchedAt := provider.client.now().UTC()
	page := capability.EventPage{Status: "complete"}
	if len(response.Events) >= request.Limit {
		page.Status = "truncated"
	}
	for _, record := range response.Events {
		event, entities, relations, mappingErr := domainEventFromRecord(record, fetchedAt, "")
		if mappingErr != nil {
			return capability.EventPage{}, translateError(mappingErr)
		}
		page.Events = append(page.Events, event)
		page.Entities = append(page.Entities, entities...)
		page.Relations = append(page.Relations, relations...)
	}
	return page, nil
}

func (provider *Provider) ResolveContext(ctx context.Context, access capability.Access, request capability.ResolveContextRequest) (capability.ContextPage, error) {
	if provider == nil || provider.client == nil {
		return capability.ContextPage{}, sourceRequestError("source_unavailable", "MaxPatrol client is not configured")
	}
	// Legacy raw refs carry no time range. The exact UUID/entity predicates keep
	// this bounded logically; the window is capped at Unix epoch through now.
	timeRange := TimeRange{From: time.Unix(0, 0).UTC(), To: provider.client.now().UTC().Add(time.Second)}
	fetchedAt := provider.client.now().UTC()
	page := capability.ContextPage{}
	missing := make([]string, 0)
	for _, externalID := range dedupeStrings(request.EventIDs) {
		record, err := provider.client.getExactEvent(ctx, Access{Cookie: access.Cookie}, "event detail", timeRange, externalID)
		if err != nil {
			if isNotFound(err) {
				missing = append(missing, externalID)
				continue
			}
			return page, translateError(err)
		}
		event, entities, relations, mappingErr := domainEventFromRecord(record, fetchedAt, "")
		if mappingErr != nil {
			return page, translateError(mappingErr)
		}
		page.Events = append(page.Events, event)
		page.Entities = append(page.Entities, entities...)
		page.Relations = append(page.Relations, relations...)
	}
	for _, sourceEntityID := range dedupeStrings(request.EntityIDs) {
		entity, parseErr := sourceEntityRef(sourceEntityID)
		if parseErr != nil {
			return page, parseErr
		}
		result, lookupErr := provider.LookupEntity(ctx, access, capability.LookupEntityRequest{
			Entity:    entity,
			TimeRange: domain.TimeRange{From: timeRange.From, To: timeRange.To},
		})
		if lookupErr != nil {
			return page, lookupErr
		}
		found := false
		for _, candidate := range result.Entities {
			for _, provenance := range candidate.Provenance {
				if provenance.Source == SourceCode && provenance.ExternalID == sourceEntityID {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			missing = append(missing, sourceEntityID)
			continue
		}
		page.Entities = append(page.Entities, result.Entities...)
		page.Relations = append(page.Relations, result.Relations...)
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return page, fmt.Errorf("%w: MaxPatrol raw records are missing", domain.ErrNotFound)
	}
	return page, nil
}

func (provider *Provider) LookupEntity(ctx context.Context, access capability.Access, request capability.LookupEntityRequest) (capability.LookupEntityResult, error) {
	if err := validateDomainTimeRange(request.TimeRange); err != nil {
		return capability.LookupEntityResult{}, err
	}
	eventRequest := capability.SearchEventsRequest{
		TimeFrom: request.TimeRange.From,
		TimeTo:   request.TimeRange.To,
		Entities: []domain.EntityRef{request.Entity},
		Limit:    100,
	}
	page, err := provider.SearchEvents(ctx, access, eventRequest)
	if err != nil {
		return capability.LookupEntityResult{}, err
	}
	return capability.LookupEntityResult{Entities: page.Entities, Relations: page.Relations, Verdicts: []domain.Verdict{}}, nil
}

func (provider *Provider) GetAccountUserinfo(ctx context.Context, access capability.Access) (domain.AccountUserinfo, error) {
	userinfo, err := provider.client.GetAccountUserinfo(ctx, Access{Cookie: access.Cookie})
	if err != nil {
		return domain.AccountUserinfo{}, translateError(err)
	}
	return domain.AccountUserinfo{SourceCode: SourceCode, UserName: userinfo.UserName}, nil
}

func (provider *Provider) Probe(ctx context.Context, access capability.Access) (string, error) {
	type probeResult struct{ err error }
	results := make(chan probeResult, 2)
	go func() {
		_, err := provider.client.GetAccountUserinfo(ctx, Access{Cookie: access.Cookie})
		results <- probeResult{err: err}
	}()
	go func() {
		now := provider.client.now().UTC()
		_, err := provider.client.SearchIncidents(ctx, Access{Cookie: access.Cookie}, IncidentSearchRequest{
			TimeRange: TimeRange{From: now.Add(-time.Hour), To: now}, Limit: 1,
		})
		results <- probeResult{err: err}
	}()
	succeeded := 0
	var firstError error
	for range 2 {
		result := <-results
		if result.err == nil {
			succeeded++
		} else if firstError == nil {
			firstError = result.err
		}
	}
	switch succeeded {
	case 2:
		return "online", nil
	case 1:
		return "degraded", nil
	default:
		return "offline", translateError(firstError)
	}
}

func findingKinds(kinds []string) (bool, bool) {
	if len(kinds) == 0 {
		return true, true
	}
	incidents, correlations := false, false
	for _, kind := range kinds {
		switch strings.TrimSpace(kind) {
		case IncidentRecordType:
			incidents = true
		case CorrelationRecordType:
			correlations = true
		}
	}
	return incidents, correlations
}

func encodeFindingCursor(cursor findingCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", sourceRequestError("invalid_cursor", "MaxPatrol cursor could not be encoded")
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeFindingCursor(raw string) (findingCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return findingCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return findingCursor{}, sourceRequestError("invalid_cursor", "MaxPatrol cursor is invalid")
	}
	var cursor findingCursor
	if json.Unmarshal(decoded, &cursor) != nil || cursor.IncidentOffset < 0 {
		return findingCursor{}, sourceRequestError("invalid_cursor", "MaxPatrol cursor is invalid")
	}
	return cursor, nil
}

func validateFindingRef(ref domain.SourceObjectRef) error {
	if ref.SourceCode != SourceCode || ref.SourceInstance != "" {
		return sourceRequestError("invalid_source_ref", "MaxPatrol source identity is invalid")
	}
	if ref.RecordType != IncidentRecordType && ref.RecordType != CorrelationRecordType {
		return sourceRequestError("invalid_source_ref", "MaxPatrol finding record_type is not supported")
	}
	if _, err := validateUUID(ref.ExternalID); err != nil {
		return sourceRequestError("invalid_source_ref", "MaxPatrol external_id must be a canonical UUID")
	}
	return validateDomainTimeRange(ref.TimeRange)
}

func validateDomainTimeRange(value domain.TimeRange) error {
	if value.From.IsZero() || value.To.IsZero() || !value.From.Before(value.To) {
		return sourceRequestError("invalid_time_range", "time_range.from must be earlier than time_range.to")
	}
	return nil
}

func sourceRequestError(code, message string) error {
	return &domain.RequestError{Code: code, Message: message}
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		return fmt.Errorf("%w: MaxPatrol record %s", domain.ErrNotFound, notFound.ExternalID)
	}
	var upstream *HTTPError
	if errors.As(err, &upstream) {
		return &domain.UpstreamError{StatusCode: upstream.StatusCode, Message: "MaxPatrol source request failed"}
	}
	var response *ResponseError
	if errors.As(err, &response) {
		return &domain.UpstreamError{StatusCode: http.StatusBadGateway, Message: "MaxPatrol source response is invalid"}
	}
	var access *AccessError
	if errors.As(err, &access) {
		return sourceRequestError("source_unavailable", "MaxPatrol credentials are unavailable")
	}
	var request *RequestError
	if errors.As(err, &request) {
		return sourceRequestError("invalid_source_request", request.Message)
	}
	return err
}

func isNotFound(err error) bool {
	var value *NotFoundError
	return errors.As(err, &value)
}

func sourceEntityRef(sourceID string) (domain.EntityRef, error) {
	kind, value, ok := strings.Cut(strings.TrimSpace(sourceID), ":")
	if !ok || strings.TrimSpace(kind) == "" || strings.TrimSpace(value) == "" {
		return domain.EntityRef{}, sourceRequestError("invalid_source_ref", "MaxPatrol entity source ID is invalid")
	}
	return domain.EntityRef{Type: kind, Value: value}, nil
}

func dedupeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func eventWhere(entities []domain.EntityRef) (string, error) {
	if len(entities) == 0 {
		return "uuid != null", nil
	}
	predicates := make([]string, 0, len(entities))
	for _, entity := range entities {
		kind := strings.ToLower(strings.TrimSpace(entity.Type))
		value := domain.CanonicalValue(kind, entity.Value)
		if value == "" || len(value) > maxIdentifierLength {
			return "", sourceRequestError("invalid_entity", "entity value is invalid")
		}
		quoted := strconv.Quote(value)
		switch kind {
		case "ip":
			if normalizeIP(value) == "" {
				return "", sourceRequestError("invalid_entity", "IP entity value is invalid")
			}
			predicates = append(predicates, "(src.ip = "+quoted+" or dst.ip = "+quoted+" or event_src.ip = "+quoted+")")
		case "host", "hostname":
			if !safeHost(value) {
				return "", sourceRequestError("invalid_entity", "host entity value is invalid")
			}
			predicates = append(predicates, "(src.host = "+quoted+" or dst.host = "+quoted+" or event_src.host = "+quoted+")")
		case "account":
			if strings.ContainsAny(value, "\r\n\x00") {
				return "", sourceRequestError("invalid_entity", "account entity value is invalid")
			}
			name, domainName := accountParts(value)
			quotedName := strconv.Quote(name)
			if domainName == "" {
				predicates = append(predicates, "(subject.account.name = "+quotedName+" or object.account.name = "+quotedName+")")
			} else {
				quotedDomain := strconv.Quote(domainName)
				predicates = append(predicates,
					"((subject.account.name = "+quotedName+" and subject.account.domain = "+quotedDomain+") or "+
						"(object.account.name = "+quotedName+" and object.account.domain = "+quotedDomain+"))",
				)
			}
		default:
			return "", sourceRequestError("unsupported_entity_type", "MaxPatrol entity type is not supported")
		}
	}
	return "(" + strings.Join(predicates, " or ") + ")", nil
}

func accountParts(value string) (name, domainName string) {
	if domainName, name, ok := strings.Cut(value, "\\"); ok {
		return name, domainName
	}
	if name, domainName, ok := strings.Cut(value, "@"); ok {
		return name, domainName
	}
	return value, ""
}

func safeHost(value string) bool {
	if len(value) < 1 || len(value) > 253 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}
