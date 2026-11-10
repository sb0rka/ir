package transport

import (
	"net/http"
	"testing"
)

func TestGatewayResponseKeepsOneBrowserCORSBoundary(t *testing.T) {
	response := &http.Response{StatusCode: 200, Header: http.Header{}}
	response.Header.Set("Access-Control-Allow-Origin", "http://localhost:5174")
	response.Header.Set("Access-Control-Allow-Credentials", "true")
	response.Header.Set("Set-Cookie", "upstream=must-not-forward")
	response.Header.Set("Content-Type", "application/json")
	if err := sanitizeGatewayResponse(response); err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Access-Control-Allow-Origin") != "" || response.Header.Get("Access-Control-Allow-Credentials") != "" || response.Header.Get("Set-Cookie") != "" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Content-Type") != "application/json" {
		t.Fatal("unsafe Gateway response headers")
	}
	response.StatusCode = 302
	if sanitizeGatewayResponse(response) == nil {
		t.Fatal("upstream redirect accepted")
	}
}
