// Package proxy contains adapters for real vendor APIs.
//
// Each product package builds a registry.Provider from process-owned config
// and implements interfaces from internal/capability. Vendor URLs, credentials,
// request types, and raw responses must not leave this subtree.
package proxy
