package maxpatrol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
)

const (
	accountUserinfoPath = "api/account/userinfo"
	maxResponseBytes    = 1 << 20
	baseURLSecretName   = "DEMO_PT_SIEM_BASE_URL"
	cookieSecretName    = "DEMO_PT_COOKIE"
)

// AccountAdapter implements the canonical account capability while keeping
// MaxPatrol request and response details inside this package.
type AccountAdapter struct{}

func (AccountAdapter) SecretNames() capability.AccountSecretNames {
	return capability.AccountSecretNames{BaseURL: baseURLSecretName, Credential: cookieSecretName}
}

func (AccountAdapter) GetAccountUserinfo(ctx context.Context, request capability.AccountUserinfoRequest) (domain.AccountUserinfo, error) {
	client, err := NewClient(proxy.HTTPClientConfig{
		BaseURL:     request.BaseURL,
		Timeout:     request.Timeout,
		SkipTLSVerify: request.SkipTLSVerify,
	}, request.Credential)
	if err != nil {
		return domain.AccountUserinfo{}, &domain.RequestError{
			Code:    "source_unavailable",
			Message: "source credentials are not configured correctly",
		}
	}
	userinfo, err := client.GetAccountUserinfo(ctx)
	if err != nil {
		return domain.AccountUserinfo{}, err
	}
	return domain.AccountUserinfo{SourceCode: SourceCode, UserName: userinfo.UserName}, nil
}

type Client struct {
	http   *proxy.HTTPClient
	cookie string
}

func NewClient(cfg proxy.HTTPClientConfig, cookie string) (*Client, error) {
	httpClient, err := proxy.NewHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return nil, fmt.Errorf("cookie is required")
	}
	return &Client{http: httpClient, cookie: cookie}, nil
}

func (client *Client) GetAccountUserinfo(ctx context.Context) (AccountUserinfo, error) {
	target := client.http.BaseURL.ResolveReference(&url.URL{Path: accountUserinfoPath})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return AccountUserinfo{}, fmt.Errorf("build account userinfo request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cookie", client.cookie)

	response, err := client.http.Client.Do(request)
	if err != nil {
		return AccountUserinfo{}, fmt.Errorf("request account userinfo: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return AccountUserinfo{}, fmt.Errorf("read account userinfo response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return AccountUserinfo{}, fmt.Errorf("account userinfo response exceeds %d bytes", maxResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return AccountUserinfo{}, &domain.UpstreamError{
			StatusCode: response.StatusCode,
			Message:    fmt.Sprintf("MaxPatrol account userinfo returned HTTP %d", response.StatusCode),
		}
	}

	var userinfo AccountUserinfo
	if err := json.Unmarshal(body, &userinfo); err != nil {
		return AccountUserinfo{}, fmt.Errorf("decode account userinfo: %w", err)
	}
	if strings.TrimSpace(userinfo.UserName) == "" {
		return AccountUserinfo{}, fmt.Errorf("account userinfo response is missing userName")
	}
	return userinfo, nil
}
