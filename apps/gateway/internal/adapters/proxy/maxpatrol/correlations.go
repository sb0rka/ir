package maxpatrol

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const eventsPath = "api/events/v3/events"

var correlationSelect = []string{
	"time",
	"uuid",
	"text",
	"action",
	"importance",
	"correlation_name",
	"correlation_type",
	"count.subevents",
	"subevents",
	"alert.context",
	"alert.key",
	"alert.regex_match",
	"event_src.host",
	"event_src.fqdn",
	"event_src.hostname",
	"event_src.ip",
	"event_src.mac",
	"event_src.vendor",
	"event_src.title",
	"event_src.subsys",
	"src.host",
	"src.hostname",
	"src.ip",
	"src.mac",
	"src.port",
	"dst.host",
	"dst.hostname",
	"dst.fqdn",
	"dst.ip",
	"dst.mac",
	"dst.port",
	"external_dst.host",
	"external_dst.hostname",
	"external_dst.fqdn",
	"subject.account.name",
	"subject.account.domain",
	"subject.account.id",
	"object.account.name",
	"object.account.domain",
	"object.account.id",
	"subject.process.name",
	"subject.process.fullpath",
	"subject.process.cmdline",
	"subject.process.chain",
	"object.process.name",
	"object.process.fullpath",
	"object.process.cmdline",
	"object.process.chain",
	"object.name",
	"object.path",
	"category.generic",
	"category.high",
	"category.low",
}

func (client *Client) SearchCorrelations(ctx context.Context, access Access, request CorrelationSearchRequest) (CorrelationPage, error) {
	if err := request.TimeRange.validate(); err != nil {
		return CorrelationPage{}, err
	}
	if request.Limit < 1 || request.Limit > defaultChildPageSize {
		return CorrelationPage{}, &RequestError{Operation: "correlation search", Message: fmt.Sprintf("limit must be between 1 and %d", defaultChildPageSize)}
	}
	response, err := client.queryEvents(ctx, access, "correlation search", request.TimeRange, "correlation_name != null", request.Limit)
	if err != nil {
		return CorrelationPage{}, err
	}
	correlations := make([]Correlation, 0, min(len(response.Events), request.Limit))
	seen := make(map[string]struct{}, len(response.Events))
	for index := range response.Events {
		correlation, mappingErr := eventToCorrelation(response.Events[index])
		if mappingErr != nil {
			return CorrelationPage{}, &ResponseError{Operation: "correlation search", Message: fmt.Sprintf("event %d is invalid", index)}
		}
		if _, exists := seen[correlation.UUID]; exists {
			continue
		}
		seen[correlation.UUID] = struct{}{}
		if len(correlations) < request.Limit {
			correlations = append(correlations, correlation)
		}
	}
	sort.Slice(correlations, func(left, right int) bool {
		if !correlations[left].OccurredAt.Equal(correlations[right].OccurredAt) {
			return correlations[left].OccurredAt.After(correlations[right].OccurredAt)
		}
		return correlations[left].UUID < correlations[right].UUID
	})
	return CorrelationPage{
		Correlations: correlations,
		TotalItems:   response.TotalCount,
		// v3 with noCount=true does not return a total; hitting the limit implies truncation.
		Truncated: len(response.Events) >= request.Limit,
	}, nil
}

func (client *Client) ResolveCorrelation(ctx context.Context, access Access, request CorrelationResolveRequest) (CorrelationResolution, error) {
	if err := request.TimeRange.validate(); err != nil {
		return CorrelationResolution{}, err
	}
	id, err := validateUUID(request.ExternalID)
	if err != nil {
		return CorrelationResolution{}, err
	}
	record, err := client.getExactEvent(ctx, access, "correlation detail", request.TimeRange, id)
	if err != nil {
		return CorrelationResolution{}, err
	}
	correlation, err := eventToCorrelation(record)
	if err != nil {
		return CorrelationResolution{}, err
	}
	return client.resolveCorrelationRecord(ctx, access, request.TimeRange, correlation), nil
}

func (client *Client) resolveCorrelationRecord(ctx context.Context, access Access, timeRange TimeRange, correlation Correlation) CorrelationResolution {
	result := CorrelationResolution{Correlation: correlation, Complete: true}
	seen := make(map[string]struct{}, len(correlation.SubeventIDs))
	for _, subeventID := range correlation.SubeventIDs {
		if _, exists := seen[subeventID]; exists {
			continue
		}
		seen[subeventID] = struct{}{}
		record, err := client.getExactEvent(ctx, access, "correlation subevent", timeRange, subeventID)
		if err != nil {
			result.Errors = append(result.Errors, contextError("correlation.subevents", err))
			continue
		}
		event, err := eventToRaw(record)
		if err != nil {
			result.Errors = append(result.Errors, ContextError{Component: "correlation.subevents", Code: "invalid_response", Message: "a correlation subevent is invalid"})
			continue
		}
		result.Subevents = append(result.Subevents, event)
	}
	if correlation.SubeventCount != len(correlation.SubeventIDs) {
		result.Errors = append(result.Errors, ContextError{
			Component: "correlation.subevents",
			Code:      "truncated",
			Message:   "the correlation did not enumerate every counted subevent",
		})
	}
	result.Complete = len(result.Errors) == 0
	return result
}

func (client *Client) getExactEvent(ctx context.Context, access Access, operation string, timeRange TimeRange, externalID string) (safeEventRecord, error) {
	id, err := validateUUID(externalID)
	if err != nil {
		return safeEventRecord{}, err
	}
	response, err := client.queryEvents(ctx, access, operation, timeRange, `uuid = "`+id+`"`, 1)
	if err != nil {
		return safeEventRecord{}, err
	}
	if response.TotalCount == 0 && len(response.Events) == 0 {
		return safeEventRecord{}, &NotFoundError{Kind: "event", ExternalID: id}
	}
	if response.TotalCount != 1 || len(response.Events) != 1 {
		return safeEventRecord{}, &ResponseError{Operation: operation, Message: "exact UUID lookup did not return one event"}
	}
	returnedID, validationErr := validateUUID(response.Events[0].UUID)
	if validationErr != nil || returnedID != id {
		return safeEventRecord{}, &ResponseError{Operation: operation, Message: "event identity does not match the request"}
	}
	return response.Events[0], nil
}

func buildEventsV3PDQL(where string, limit int) string {
	if limit < 1 {
		limit = 1
	}
	where = strings.TrimSpace(where)
	if where == "" {
		where = "uuid != null"
	}
	return fmt.Sprintf(
		"filter(%s) | select(%s) | sort(time desc) | limit(%d)",
		where,
		strings.Join(correlationSelect, ", "),
		limit,
	)
}

func (client *Client) queryEvents(ctx context.Context, access Access, operation string, timeRange TimeRange, where string, top int) (safeEventsEnvelope, error) {
	if err := timeRange.validate(); err != nil {
		return safeEventsEnvelope{}, err
	}
	request := eventQueryV3Request{
		Filter:      buildEventsV3PDQL(where, top),
		GroupValues: []string{"1"},
		TimeFrom:    timeRange.From.Unix(),
		TimeTo:      timeRange.To.Unix(),
	}
	query := url.Values{}
	query.Set("offset", "0")
	query.Set("limit", fmt.Sprintf("%d", top))
	query.Set("recursive", "true")
	query.Set("noCount", "true")
	var response safeEventsEnvelope
	if err := client.doJSON(ctx, client.siem, access, operation, http.MethodPost, eventsPath, query, request, &response); err != nil {
		return safeEventsEnvelope{}, err
	}
	if nonNullVendorErrors(response.Errors) {
		return safeEventsEnvelope{}, &ResponseError{Operation: operation, Message: "vendor reported query errors"}
	}
	if response.TotalCount <= 0 {
		response.TotalCount = int64(len(response.Events))
	}
	if int64(len(response.Events)) > response.TotalCount {
		return safeEventsEnvelope{}, &ResponseError{Operation: operation, Message: "pagination metadata is inconsistent"}
	}
	return response, nil
}
