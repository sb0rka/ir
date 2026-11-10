package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

func settingsFingerprint(values ...any) string {
	raw, _ := json.Marshal(values)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ConfigurationFingerprint binds event cursors to server-owned source settings.
func ConfigurationFingerprint(configured map[string]ConfiguredSource, options BuildOptions) string {
	codes := make([]string, 0, len(configured))
	for code := range configured {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	type entry struct {
		Code        string
		Instances   []string
		Fingerprint string
	}
	entries := make([]entry, 0, len(codes))
	for _, code := range codes {
		source := configured[code]
		entries = append(entries, entry{code, source.InstanceIDs, source.Fingerprint})
	}
	return settingsFingerprint(entries, options)
}
