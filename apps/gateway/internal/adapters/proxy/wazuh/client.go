package wazuh

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
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
)

const SourceCode = "wazuh"
const CredentialSelector = "WAZUH_BASIC"
const pitKeepAlive = "5m"
const maxResponseBytes = 8 << 20

type ClientConfig struct {
	HTTP            proxy.HTTPClientConfig
	IndexPattern    string
	DocumentIDField string
	Now             func() time.Time
}

type Client struct {
	http            *proxy.HTTPClient
	indexPattern    string
	documentIDField string
	now             func() time.Time
}

type pointInTime struct {
	ID        string `json:"id"`
	KeepAlive string `json:"keep_alive"`
}

var indexPatternSyntax = regexp.MustCompile(`^[a-z0-9][a-z0-9.*_-]*$`)
var documentFieldSyntax = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.IndexPattern == "" {
		cfg.IndexPattern = "wazuh-alerts-*"
	}
	if cfg.DocumentIDField == "" {
		cfg.DocumentIDField = "_id"
	}
	if err := ValidateIndexSettings(cfg.IndexPattern, cfg.DocumentIDField); err != nil {
		return nil, err
	}
	backend, err := proxy.NewHTTPClient(cfg.HTTP)
	if err != nil {
		return nil, err
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Client{http: backend, indexPattern: cfg.IndexPattern, documentIDField: cfg.DocumentIDField, now: cfg.Now}, nil
}

func (client *Client) doJSON(ctx context.Context, access capability.Access, method, endpoint string, query url.Values, body, dest any) error {
	if access.Basic == nil || access.Cookie != "" {
		return sourceRequestError("source_unavailable", "source credentials are not configured for this project")
	}
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	target := client.http.BaseURL.ResolveReference(&url.URL{Path: endpoint, RawQuery: query.Encode()})
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(encoded))
	if err != nil {
		return &ResponseError{Message: "invalid source request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(access.Basic.Username, access.Basic.Password)
	response, err := client.http.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var ne net.Error
		result := &TransportError{Operation: "request"}
		if errors.As(err, &ne) {
			result.TimedOut = ne.Timeout()
			result.TemporaryFailure = ne.Temporary()
		}
		return result
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &domain.UpstreamError{StatusCode: response.StatusCode, Message: "Wazuh source request failed"}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &TransportError{Operation: "read"}
	}
	if len(raw) > maxResponseBytes {
		return &domain.UpstreamError{StatusCode: 502, Message: "Wazuh response is too large"}
	}
	if dest == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decodeErr := decoder.Decode(dest); decodeErr != nil {
		var mismatch *json.UnmarshalTypeError
		if errors.As(decodeErr, &mismatch) {
			return &domain.UpstreamError{StatusCode: 502, Message: "Wazuh response field type is invalid: " + mismatch.Field}
		}
		return &domain.UpstreamError{StatusCode: 502, Message: "Wazuh response is invalid"}
	}
	if decoder.Decode(new(any)) != io.EOF {
		return &domain.UpstreamError{StatusCode: 502, Message: "Wazuh response is invalid"}
	}
	return nil
}

func (client *Client) openPIT(ctx context.Context, access capability.Access) (string, error) {
	var response struct {
		ID     string `json:"pit_id"`
		Shards struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
	}
	err := client.doJSON(ctx, access, http.MethodPost, client.indexPattern+"/_search/point_in_time", url.Values{"keep_alive": {pitKeepAlive}}, nil, &response)
	if err != nil {
		return "", err
	}
	if response.ID == "" || len(response.ID) > 128<<10 || response.Shards.Failed != 0 {
		if response.ID != "" {
			_ = client.closePIT(ctx, access, response.ID)
		}
		return "", &domain.UpstreamError{StatusCode: 502, Message: "Wazuh snapshot is unavailable"}
	}
	return response.ID, nil
}

func (client *Client) closePIT(ctx context.Context, access capability.Access, id string) error {
	return client.doJSON(ctx, access, http.MethodDelete, "_search/point_in_time", nil, struct {
		IDs []string `json:"pit_id"`
	}{[]string{id}}, nil)
}

func (client *Client) search(ctx context.Context, access capability.Access, body searchRequest) (searchResponse, error) {
	var response searchResponse
	endpoint := client.indexPattern + "/_search"
	if body.PIT != nil {
		endpoint = "_search"
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline).Milliseconds()
		if remaining > 0 {
			body.Timeout = fmt.Sprintf("%dms", remaining)
		}
	}
	err := client.doJSON(ctx, access, http.MethodPost, endpoint, nil, body, &response)
	var upstream *domain.UpstreamError
	if body.PIT != nil && errors.As(err, &upstream) && upstream.StatusCode == 404 {
		return response, sourceRequestError("invalid_cursor", "source search snapshot expired; start a new search")
	}
	return response, err
}

func (client *Client) validateIndex(index string) error {
	matched, err := path.Match(client.indexPattern, index)
	if err != nil || !matched || strings.ContainsAny(index, "/\\,*?[]\r\n\x00") || strings.Contains(index, "..") {
		return sourceRequestError("invalid_source_ref", "source event index is outside the configured alerts pattern")
	}
	return nil
}

func (client *Client) validateRef(id string) (string, string, error) {
	index, document, ok := strings.Cut(id, "/")
	if !ok || document == "" || len(document) > 512 || strings.ContainsAny(document, "\r\n\x00") {
		return "", "", sourceRequestError("invalid_source_ref", "Wazuh source event ID is invalid")
	}
	if err := client.validateIndex(index); err != nil {
		return "", "", err
	}
	return index, document, nil
}

func sourceRequestError(code, message string) error {
	return &domain.RequestError{Code: code, Message: message}
}
func translateError(err error) error {
	var request *RequestError
	if errors.As(err, &request) {
		return sourceRequestError("invalid_source_request", request.Message)
	}
	var response *ResponseError
	if errors.As(err, &response) {
		return &domain.UpstreamError{StatusCode: 502, Message: "Wazuh source response is invalid"}
	}
	return err
}

func ValidateIndexSettings(pattern, documentField string) error {
	if !indexPatternSyntax.MatchString(pattern) || strings.Contains(pattern, "..") {
		return errors.New("invalid Wazuh alerts index pattern")
	}
	if !documentFieldSyntax.MatchString(documentField) {
		return errors.New("invalid Wazuh document ID field")
	}
	return nil
}
