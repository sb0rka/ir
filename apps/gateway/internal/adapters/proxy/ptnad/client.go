package ptnad

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes int64 = 8 << 20

const (
	nestedHTTPPageSize     = 100
	maxNestedHTTPPageCalls = MaxLimit / nestedHTTPPageSize
)

type Config struct {
	BaseURL    string
	HTTPClient *http.Client
	Now        func() time.Time
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
	now     func() time.Time
}

func NewClient(config Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, fmt.Errorf("PT NAD base URL must be an absolute HTTP(S) URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("PT NAD base URL must not contain credentials, query, or fragment")
	}
	if config.HTTPClient == nil {
		return nil, fmt.Errorf("PT NAD HTTP client is required")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/"

	// Keep the caller-owned transport and timeout, but never let a credentialed
	// request follow a redirect to this or another origin.
	httpClient := *config.HTTPClient
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Client{baseURL: baseURL, http: &httpClient, now: now}, nil
}

func (client *Client) SearchSessions(ctx context.Context, request SearchRequest, access Access) (SessionSearchResult, error) {
	request, timeRange, err := validateSearchRequest(request)
	if err != nil {
		return SessionSearchResult{}, err
	}
	body := sessionListBQL(request)
	var response sessionListResponse
	if err := client.doJSON(ctx, "session search", http.MethodPost, "api/v2/bql", sourceQuery(request.StoreID), body, access, &response); err != nil {
		return SessionSearchResult{}, err
	}
	if err := validateBQLPage(response.Total, len(response.Result)); err != nil {
		return SessionSearchResult{}, fmt.Errorf("decode PT NAD session search: %w", err)
	}

	fetchedAt := client.now().UTC()
	result := SessionSearchResult{Total: response.Total, Sessions: make([]Session, 0, len(response.Result))}
	seen := make(map[string]struct{}, len(response.Result))
	for index, row := range response.Result {
		session, mapErr := mapSessionRow(row, request.StoreID, timeRange, fetchedAt)
		if mapErr != nil {
			return SessionSearchResult{}, fmt.Errorf("map PT NAD session row %d: %w", index, mapErr)
		}
		identity := session.SourceRef.Identity()
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		result.Sessions = append(result.Sessions, session)
	}
	result.Truncated = int64(len(result.Sessions)) < response.Total
	return result, nil
}

func (client *Client) SearchAttacks(ctx context.Context, request SearchRequest, access Access) (AttackSearchResult, error) {
	request, timeRange, err := validateSearchRequest(request)
	if err != nil {
		return AttackSearchResult{}, err
	}
	return client.searchAttacks(ctx, request, timeRange, "", access)
}

// GetAttack uses an exact, escaped ID predicate. It never obtains a broad page
// and filters it client-side.
func (client *Client) GetAttack(ctx context.Context, ref AttackRef, access Access) (Attack, error) {
	request, timeRange, err := validateAttackRef(ref)
	if err != nil {
		return Attack{}, err
	}
	result, err := client.searchAttacks(ctx, request, timeRange, ref.ExternalID, access)
	if err != nil {
		return Attack{}, err
	}
	for _, attack := range result.Attacks {
		if attack.SourceRef.ExternalID == ref.ExternalID {
			return attack, nil
		}
	}
	if result.Total == 0 {
		return Attack{}, ErrNotFound
	}
	return Attack{}, fmt.Errorf("PT NAD exact attack response did not contain the requested ID")
}

func (client *Client) searchAttacks(ctx context.Context, request SearchRequest, timeRange TimeRange, exactID string, access Access) (AttackSearchResult, error) {
	body := attackListBQL(request, exactID)
	var response attackListResponse
	if err := client.doJSON(ctx, "attack search", http.MethodPost, "api/v2/bql", sourceQuery(request.StoreID), body, access, &response); err != nil {
		return AttackSearchResult{}, err
	}
	if err := validateBQLPage(response.Total, len(response.Result)); err != nil {
		return AttackSearchResult{}, fmt.Errorf("decode PT NAD attack search: %w", err)
	}

	fetchedAt := client.now().UTC()
	result := AttackSearchResult{Total: response.Total, Attacks: make([]Attack, 0, len(response.Result))}
	seen := make(map[string]struct{}, len(response.Result))
	for index, row := range response.Result {
		attack, mapErr := mapAttackRow(row, request.StoreID, timeRange, fetchedAt)
		if mapErr != nil {
			return AttackSearchResult{}, fmt.Errorf("map PT NAD attack row %d: %w", index, mapErr)
		}
		identity := attack.SourceRef.Identity()
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		result.Attacks = append(result.Attacks, attack)
	}
	result.Truncated = int64(len(result.Attacks)) < response.Total
	return result, nil
}

func (client *Client) GetSession(ctx context.Context, ref SessionRef, access Access) (Session, error) {
	ref, err := validateSessionRef(ref)
	if err != nil {
		return Session{}, err
	}
	rawQuery := "start=" + strconv.FormatInt(ref.TimeRange.From.UnixMilli(), 10) +
		"&end=" + strconv.FormatInt(ref.TimeRange.To.UnixMilli(), 10) +
		"&source=" + strconv.FormatInt(ref.StoreID, 10)
	var detail flowDetail
	if err := client.doJSON(ctx, "session detail", http.MethodGet, "api/v2/flow/"+ref.ExternalID, rawQuery, "", access, &detail); err != nil {
		return Session{}, err
	}
	if strings.TrimSpace(detail.ID) == "" {
		return Session{}, ErrNotFound
	}
	if detail.ID != ref.ExternalID {
		return Session{}, fmt.Errorf("PT NAD session detail ID does not match the requested ID")
	}
	contextErr := client.completeHTTPTransactions(ctx, ref, access, &detail)
	session, err := mapFlowDetail(detail, ref.StoreID, ref.TimeRange, client.now().UTC())
	if err != nil {
		return Session{}, err
	}
	if contextErr != nil {
		session.ContextErrors = append(session.ContextErrors, contextErr)
	}
	return session, nil
}

func (client *Client) completeHTTPTransactions(ctx context.Context, ref SessionRef, access Access, detail *flowDetail) error {
	if len(detail.HTTP) < nestedHTTPPageSize {
		return nil
	}

	seen := make(map[string]struct{}, len(detail.HTTP))
	lastTxID := int64(-1)
	for _, transaction := range detail.HTTP {
		seen[transaction.ID] = struct{}{}
		if transaction.TxID > lastTxID {
			lastTxID = transaction.TxID
		}
	}
	if lastTxID < 0 {
		return &ProtocolError{Operation: "session HTTP pagination"}
	}

	for page := 0; page < maxNestedHTTPPageCalls; page++ {
		fromTxID := lastTxID + 1
		var response httpPageResponse
		if err := client.doJSON(
			ctx,
			"session HTTP pagination",
			http.MethodPost,
			"api/v2/bql",
			sourceQuery(ref.StoreID),
			httpPageBQL(ref, fromTxID),
			access,
			&response,
		); err != nil {
			return err
		}
		if err := validateBQLPage(response.Total, len(response.Result)); err != nil || response.Total != 1 || len(response.Result) != 1 || response.Result[0].SessionID != ref.ExternalID {
			return &ProtocolError{Operation: "session HTTP pagination"}
		}
		if len(response.Result[0].Transactions) == 0 {
			return nil
		}

		progressed := false
		for _, transaction := range response.Result[0].Transactions {
			if transaction.TxID < fromTxID {
				return &ProtocolError{Operation: "session HTTP pagination"}
			}
			value := transaction.detail(ref.ExternalID)
			if _, err := mapTransaction(value.transactionDTO, ref.ExternalID); err != nil {
				return &ProtocolError{Operation: "session HTTP pagination"}
			}
			if transaction.TxID > lastTxID {
				lastTxID = transaction.TxID
				progressed = true
			}
			if _, duplicate := seen[transaction.ID]; duplicate {
				continue
			}
			seen[transaction.ID] = struct{}{}
			detail.HTTP = append(detail.HTTP, value)
		}
		if !progressed {
			return &ProtocolError{Operation: "session HTTP pagination"}
		}
	}
	return &ProtocolError{Operation: "session HTTP pagination"}
}

func (client *Client) GetStore(ctx context.Context, storeID int64, access Access) (Store, error) {
	if storeID <= 0 {
		return Store{}, fmt.Errorf("PT NAD store ID must be positive")
	}
	var detail storeDetail
	if err := client.doJSON(ctx, "store detail", http.MethodGet, "api/v2/sources/"+strconv.FormatInt(storeID, 10), "", "", access, &detail); err != nil {
		return Store{}, err
	}
	if detail.ID != storeID {
		return Store{}, fmt.Errorf("PT NAD store detail ID does not match the requested store")
	}
	return mapStore(detail, client.now().UTC())
}

func (client *Client) doJSON(ctx context.Context, operation, method, relativePath, rawQuery, body string, access Access, target any) error {
	cookie, err := validateAccess(access)
	if err != nil {
		return err
	}
	endpoint := client.baseURL.ResolveReference(&url.URL{Path: relativePath, RawQuery: rawQuery})
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewBufferString(body))
	if err != nil {
		return fmt.Errorf("build PT NAD %s request: %w", operation, err)
	}
	request.Header.Set("Cookie", cookie)
	if method == http.MethodPost {
		request.Header.Set("Accept", "application/json, text/plain, */*")
		request.Header.Set("Content-Type", "text/plain")
		if csrfToken, ok := cookieValue(cookie, "csrftoken"); ok {
			request.Header.Set("X-CSRFToken", csrfToken)
			request.Header.Set("Referer", client.baseURL.String())
		}
	} else if operation == "store detail" {
		request.Header.Set("Accept", "*/*")
	} else {
		request.Header.Set("Accept", "application/json, text/plain, */*")
	}

	response, err := client.http.Do(request)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		transportError := &TransportError{Operation: operation}
		var networkError net.Error
		if errors.As(err, &networkError) {
			transportError.TimedOut = networkError.Timeout()
			transportError.TemporaryFailure = networkError.Temporary()
		}
		return transportError
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &ResponseError{Operation: operation, StatusCode: response.StatusCode}
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return &TransportError{Operation: operation}
	}
	if int64(len(payload)) > maxResponseBytes {
		return &ProtocolError{Operation: operation}
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return &ProtocolError{Operation: operation}
	}
	return nil
}

func validateAccess(access Access) (string, error) {
	cookie := strings.TrimSpace(access.Cookie)
	if cookie == "" {
		return "", fmt.Errorf("PT NAD cookie is required")
	}
	if strings.ContainsAny(cookie, "\r\n") {
		return "", fmt.Errorf("PT NAD cookie contains a newline")
	}
	return cookie, nil
}

func cookieValue(cookieHeader, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false
	}
	for _, part := range strings.Split(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == name {
			value = strings.TrimSpace(value)
			if value != "" {
				return value, true
			}
		}
	}
	return "", false
}

func validateSearchRequest(request SearchRequest) (SearchRequest, TimeRange, error) {
	if request.StoreID <= 0 {
		return SearchRequest{}, TimeRange{}, fmt.Errorf("PT NAD store ID must be positive")
	}
	if request.From.IsZero() || request.To.IsZero() || !request.From.Before(request.To) {
		return SearchRequest{}, TimeRange{}, fmt.Errorf("PT NAD time range must satisfy from < to")
	}
	if request.Limit == 0 {
		request.Limit = DefaultLimit
	}
	if request.Limit < 1 || request.Limit > MaxLimit {
		return SearchRequest{}, TimeRange{}, fmt.Errorf("PT NAD limit must be between 1 and %d", MaxLimit)
	}
	request.From = request.From.UTC()
	request.To = request.To.UTC()
	return request, TimeRange{From: request.From, To: request.To}, nil
}

func validateSessionRef(ref SessionRef) (SessionRef, error) {
	if ref.StoreID <= 0 {
		return SessionRef{}, fmt.Errorf("PT NAD store ID must be positive")
	}
	if err := validateExternalID(ref.ExternalID); err != nil {
		return SessionRef{}, err
	}
	timeRange, err := validateTimeRange(ref.TimeRange)
	if err != nil {
		return SessionRef{}, err
	}
	ref.TimeRange = timeRange
	return ref, nil
}

func validateAttackRef(ref AttackRef) (SearchRequest, TimeRange, error) {
	if err := validateExternalID(ref.ExternalID); err != nil {
		return SearchRequest{}, TimeRange{}, err
	}
	request := SearchRequest{StoreID: ref.StoreID, From: ref.TimeRange.From, To: ref.TimeRange.To, Limit: 2}
	return validateSearchRequest(request)
}

func validateTimeRange(value TimeRange) (TimeRange, error) {
	if value.From.IsZero() || value.To.IsZero() || !value.From.Before(value.To) {
		return TimeRange{}, fmt.Errorf("PT NAD time range must satisfy from < to")
	}
	return TimeRange{From: value.From.UTC(), To: value.To.UTC()}, nil
}

func validateExternalID(value string) error {
	if value == "" || len(value) > 256 {
		return fmt.Errorf("PT NAD external ID is invalid")
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("_-~.", char) {
			continue
		}
		return fmt.Errorf("PT NAD external ID is invalid")
	}
	return nil
}

func validateBQLPage(total int64, returned int) error {
	if total < 0 {
		return fmt.Errorf("total is negative")
	}
	if int64(returned) > total {
		return fmt.Errorf("returned rows exceed total")
	}
	return nil
}

func sourceQuery(storeID int64) string {
	return "source=" + strconv.FormatInt(storeID, 10)
}

func sessionListBQL(request SearchRequest) string {
	return fmt.Sprintf(`SELECT "app_proto", "bytes.recv", "bytes.sent", "criticality", "dst.dns", "dst.geo.country", "dst.host_id", "dst.ip", "dst.port", "end", "errors", "false_positive", "flags", "has_files", "id", "proto", "rpt.cat", "rpt.color", "rpt.id", "rpt.type", "rpt.where", "src.dns", "src.geo.country", "src.host_id", "src.ip", "src.port", "stag", "start", "state"
FROM "flow"
WHERE
    "end" >= %d AND
    "end" <= %d
    
ORDER BY "start" desc
LIMIT %d
`, request.From.UnixMilli(), request.To.UnixMilli(), request.Limit)
}

func attackListBQL(request SearchRequest, exactID string) string {
	exact := ""
	if exactID != "" {
		exact = fmt.Sprintf("    \"id\" == '%s' AND\n", exactID)
	}
	return fmt.Sprintf(`SELECT "attacker.geo.country", "attacker.host_id", "attacker.ip", "cls", extract_raw_object('false_positive'), "id", "msg", "pr", "rev", "sid", "success.affected", "ts", "victim.geo.country", "victim.host_id", "victim.ip", (SELECT "app_proto", "dst.ip", "dst.port", "end", "flags", "has_files", "id", "rpt.cat", "rpt.color", "rpt.id", "rpt.type", "rpt.where", "src.ip", "src.port", "start", "state" FROM "flow" LIMIT 1)
FROM "alert"
WHERE
%s    "ts" >= %d AND
    "ts" <= %d AND
    EXISTS (SELECT * FROM "flow" WHERE "end" >= %d AND "end" <= %d )
ORDER BY "ts" desc
LIMIT %d
`, exact, request.From.UnixMilli(), request.To.UnixMilli(), request.From.UnixMilli(), request.To.UnixMilli(), request.Limit)
}

func httpPageBQL(ref SessionRef, fromTxID int64) string {
	return fmt.Sprintf(`SELECT "id", (SELECT "id", "tx_id", "tx_time", "rqs.method", "rqs.url", "rqs.entity_len", "rqs.content-type", "rqs.host", "rsp.code", "rsp.status", "rsp.entity_len", "rsp.server", "rsp.content-type" FROM "http" WHERE "tx_id" >= %d LIMIT %d)
FROM "flow"
WHERE
    "end" >= %d AND
    "end" <= %d AND
    "id" == '%s'
LIMIT 1
`, fromTxID, nestedHTTPPageSize, ref.TimeRange.From.UnixMilli(), ref.TimeRange.To.UnixMilli(), ref.ExternalID)
}
