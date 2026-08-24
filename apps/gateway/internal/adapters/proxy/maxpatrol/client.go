package maxpatrol

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
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
)

const (
	accountUserinfoPath  = "api/account/userinfo"
	maxResponseBytes     = int64(4 << 20)
	defaultChildPageSize = 1000
	defaultMaxChildItems = 10_000
)

// Access is supplied for each vendor call so a Client never retains a
// project credential. Callers may therefore invalidate and reload one
// project/source credential without rebuilding process-owned transports.
type Access struct {
	Cookie string
}

func (access Access) cookieHeader() (string, error) {
	if strings.ContainsAny(access.Cookie, "\r\n") {
		return "", &AccessError{Message: "cookie contains a forbidden line break"}
	}
	cookie := strings.TrimSpace(access.Cookie)
	if cookie == "" {
		return "", &AccessError{Message: "cookie is required"}
	}
	for _, character := range cookie {
		if character < 0x20 || character == 0x7f {
			return "", &AccessError{Message: "cookie contains a forbidden control character"}
		}
	}
	return cookie, nil
}

// ClientConfig contains only process-owned transport settings. SIEM serves
// event/account APIs while Incidents serves the Incident Read Model API.
type ClientConfig struct {
	SIEM             proxy.HTTPClientConfig
	Incidents        proxy.HTTPClientConfig
	MaxResponseBytes int64
	ChildPageSize    int
	MaxChildItems    int
	Now              func() time.Time
}

// AccountAdapter maps the real client to the canonical account capability.
type AccountAdapter struct {
	Client *Client
}

func (adapter AccountAdapter) GetAccountUserinfo(ctx context.Context, access capability.Access) (domain.AccountUserinfo, error) {
	if adapter.Client == nil {
		return domain.AccountUserinfo{}, &RequestError{Operation: "account userinfo", Message: "client is not configured"}
	}
	userinfo, err := adapter.Client.GetAccountUserinfo(ctx, Access{Cookie: access.Cookie})
	if err != nil {
		return domain.AccountUserinfo{}, err
	}
	return domain.AccountUserinfo{SourceCode: SourceCode, UserName: userinfo.UserName}, nil
}

type Client struct {
	siem             *proxy.HTTPClient
	incidents        *proxy.HTTPClient
	maxResponseBytes int64
	childPageSize    int
	maxChildItems    int
	internalHosts    map[string]struct{}
	now              func() time.Time
}

func NewClient(cfg ClientConfig) (*Client, error) {
	siem, err := newBackend(cfg.SIEM)
	if err != nil {
		return nil, fmt.Errorf("configure MaxPatrol SIEM transport: %w", err)
	}
	incidents, err := newBackend(cfg.Incidents)
	if err != nil {
		return nil, fmt.Errorf("configure MaxPatrol incident transport: %w", err)
	}

	responseLimit := cfg.MaxResponseBytes
	if responseLimit == 0 {
		responseLimit = maxResponseBytes
	}
	if responseLimit < 1 {
		return nil, fmt.Errorf("max response bytes must be positive")
	}
	pageSize := cfg.ChildPageSize
	if pageSize == 0 {
		pageSize = defaultChildPageSize
	}
	if pageSize < 1 || pageSize > defaultChildPageSize {
		return nil, fmt.Errorf("child page size must be between 1 and %d", defaultChildPageSize)
	}
	itemLimit := cfg.MaxChildItems
	if itemLimit == 0 {
		itemLimit = defaultMaxChildItems
	}
	if itemLimit < pageSize {
		return nil, fmt.Errorf("max child items must be at least the child page size")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &Client{
		siem:             siem,
		incidents:        incidents,
		maxResponseBytes: responseLimit,
		childPageSize:    pageSize,
		maxChildItems:    itemLimit,
		now:              now,
		internalHosts: map[string]struct{}{
			strings.ToLower(siem.BaseURL.Host):      {},
			strings.ToLower(incidents.BaseURL.Host): {},
		},
	}, nil
}

func newBackend(cfg proxy.HTTPClientConfig) (*proxy.HTTPClient, error) {
	backend, err := proxy.NewHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	// Credentials must never follow a redirect. ErrUseLastResponse makes the
	// 3xx response visible to doJSON, which turns it into a bounded HTTPError.
	httpClient := *backend.Client
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	backend.Client = &httpClient
	return backend, nil
}

func (client *Client) GetAccountUserinfo(ctx context.Context, access Access) (AccountUserinfo, error) {
	var userinfo AccountUserinfo
	if err := client.doJSON(ctx, client.siem, access, "account userinfo", http.MethodGet, accountUserinfoPath, nil, nil, &userinfo); err != nil {
		return AccountUserinfo{}, err
	}
	userinfo.UserName = cleanText(userinfo.UserName, maxNameLength)
	if userinfo.UserName == "" {
		return AccountUserinfo{}, &ResponseError{Operation: "account userinfo", Message: "userName is missing"}
	}
	// Roles and vendor identifiers are intentionally not copied to canonical
	// output by AccountAdapter.
	return userinfo, nil
}

func (client *Client) doJSON(
	ctx context.Context,
	backend *proxy.HTTPClient,
	access Access,
	operation string,
	method string,
	requestPath string,
	query url.Values,
	payload any,
	destination any,
) error {
	cookie, err := access.cookieHeader()
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return &RequestError{Operation: operation, Message: "request payload is invalid"}
		}
		body = bytes.NewReader(encoded)
	}

	reference := &url.URL{Path: requestPath}
	if len(query) > 0 {
		reference.RawQuery = query.Encode()
	}
	target := backend.BaseURL.ResolveReference(reference)
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return &RequestError{Operation: operation, Message: "request could not be built"}
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Cookie", cookie)

	response, err := backend.Client.Do(request)
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

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, client.maxResponseBytes+1))
	if err != nil {
		return &TransportError{Operation: operation}
	}
	if int64(len(responseBody)) > client.maxResponseBytes {
		return &ResponseError{Operation: operation, Message: "response exceeds the configured size limit"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &HTTPError{Operation: operation, StatusCode: response.StatusCode}
	}
	if destination == nil {
		return nil
	}
	if len(bytes.TrimSpace(responseBody)) == 0 {
		return &ResponseError{Operation: operation, Message: "response body is empty"}
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		// Do not wrap the JSON error: some decoder errors quote response data.
		return &ResponseError{Operation: operation, Message: "response is not valid JSON"}
	}
	return nil
}
