package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type HTTPClientConfig struct {
	BaseURL        string
	CredentialFile string
	Timeout        time.Duration
	TLSCAFile      string
}

type HTTPClient struct {
	BaseURL    *url.URL
	Credential string
	Client     *http.Client
}

func NewHTTPClient(cfg HTTPClientConfig) (*HTTPClient, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, fmt.Errorf("base_url must be an absolute HTTP(S) URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("base_url must not contain credentials, query, or fragment")
	}
	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}

	credential := ""
	if cfg.CredentialFile != "" {
		raw, readErr := os.ReadFile(cfg.CredentialFile)
		if readErr != nil {
			return nil, fmt.Errorf("read credential file: %w", readErr)
		}
		credential = strings.TrimSpace(string(raw))
		if credential == "" {
			return nil, fmt.Errorf("credential file is empty")
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.TLSCAFile != "" {
		raw, readErr := os.ReadFile(cfg.TLSCAFile)
		if readErr != nil {
			return nil, fmt.Errorf("read TLS CA file: %w", readErr)
		}
		roots, rootErr := x509.SystemCertPool()
		if rootErr != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(raw) {
			return nil, fmt.Errorf("TLS CA file contains no certificates")
		}
		transport.TLSClientConfig.RootCAs = roots
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/"
	return &HTTPClient{
		BaseURL:    baseURL,
		Credential: credential,
		Client:     &http.Client{Transport: transport, Timeout: cfg.Timeout},
	}, nil
}
