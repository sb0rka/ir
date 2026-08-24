// Package ptnad implements the typed PT NAD vendor boundary for sessions,
// attacks, flow details, and configured-store probes. It exposes only fixed BQL
// operations and sanitized normalized records; credentials remain per-call and
// raw vendor responses never cross the package boundary.
package ptnad
