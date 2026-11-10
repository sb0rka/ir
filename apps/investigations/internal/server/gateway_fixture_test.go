package server

import "context"

// Domain tests use a local Gateway fixture. Authentication and token exchange
// are exercised separately by the transport and live integration checks.
func fixtureGatewayBearer(_ context.Context, bearer, _ string) (string, error) { return bearer, nil }
