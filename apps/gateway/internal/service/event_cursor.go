package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/fieldmap"
)

const eventCursorTTL = 5 * time.Minute
const maxEventCursorBytes = 512 << 10

type Option func(*Service)

func WithEventPagination(key []byte, binding string) Option {
	return func(service *Service) {
		service.eventCursorKey = append([]byte(nil), key...)
		service.eventCursorBinding = binding
	}
}

type eventPosition struct {
	PageCursor string `json:"cursor,omitempty"`
	PageSize   int    `json:"size"`
	Consumed   int    `json:"consumed,omitempty"`
	Done       bool   `json:"done,omitempty"`
	Status     string `json:"status,omitempty"`
	Total      *int64 `json:"total,omitempty"`
}
type eventCursorState struct {
	Version     int                       `json:"v"`
	Project     string                    `json:"project"`
	Fingerprint string                    `json:"query"`
	Backend     string                    `json:"backend"`
	Expires     int64                     `json:"expires"`
	Sources     map[string]*eventPosition `json:"sources"`
}

func invalidEventCursor() error {
	return &domain.RequestError{Code: "invalid_cursor", Message: "cursor is invalid or expired; start a new search"}
}
func (service *Service) decodeEventCursor(raw, project, fingerprint string) (eventCursorState, error) {
	state := eventCursorState{Version: 2, Project: project, Fingerprint: fingerprint, Backend: service.eventCursorBinding, Sources: map[string]*eventPosition{}}
	if raw == "" {
		return state, nil
	}
	if len(raw) > maxEventCursorBytes || len(service.eventCursorKey) != 32 {
		return eventCursorState{}, invalidEventCursor()
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] != "ev2" {
		return eventCursorState{}, invalidEventCursor()
	}
	mac, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return eventCursorState{}, invalidEventCursor()
	}
	signer := hmac.New(sha256.New, service.eventCursorSubkey("signature"))
	signer.Write([]byte("gateway-events-v2\x00" + parts[1]))
	if !hmac.Equal(mac, signer.Sum(nil)) {
		return eventCursorState{}, invalidEventCursor()
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return eventCursorState{}, invalidEventCursor()
	}
	aead, err := service.eventCursorAEAD()
	if err != nil || len(payload) < aead.NonceSize() {
		return eventCursorState{}, invalidEventCursor()
	}
	payload, err = aead.Open(nil, payload[:aead.NonceSize()], payload[aead.NonceSize():], []byte("gateway-events-v2"))
	if err != nil {
		return eventCursorState{}, invalidEventCursor()
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil || decoder.Decode(new(any)) != io.EOF || state.Version != 2 || state.Project != project || state.Fingerprint != fingerprint || state.Backend != service.eventCursorBinding || state.Expires <= time.Now().Unix() || state.Expires > time.Now().Add(eventCursorTTL+time.Minute).Unix() || state.Sources == nil {
		return eventCursorState{}, invalidEventCursor()
	}
	for _, position := range state.Sources {
		if position == nil || position.PageSize < 1 || position.PageSize > 100 || position.Consumed < 0 || position.Consumed > position.PageSize || position.Total != nil && *position.Total < 0 || position.Status != "" && position.Status != "complete" && position.Status != "truncated" {
			return eventCursorState{}, invalidEventCursor()
		}
	}
	return state, nil
}
func (service *Service) encodeEventCursor(state eventCursorState) (string, error) {
	if len(service.eventCursorKey) != 32 {
		return "", &domain.RequestError{Code: "source_unavailable", Message: "event pagination is not configured"}
	}
	state.Expires = time.Now().Add(eventCursorTTL).Unix()
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return service.sealEventCursor(raw)
}

func (service *Service) eventCursorSubkey(purpose string) []byte {
	derive := hmac.New(sha256.New, service.eventCursorKey)
	derive.Write([]byte("gateway-events-v2/" + purpose))
	return derive.Sum(nil)
}
func (service *Service) eventCursorAEAD() (cipher.AEAD, error) {
	block, err := aes.NewCipher(service.eventCursorSubkey("encryption"))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt native sort values: an allowed field may contain text that the event
// normalizer redacts. Signing alone would expose that text through Base64.
func (service *Service) sealEventCursor(raw []byte) (string, error) {
	aead, err := service.eventCursorAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, raw, []byte("gateway-events-v2"))
	payload := base64.RawURLEncoding.EncodeToString(sealed)
	signer := hmac.New(sha256.New, service.eventCursorSubkey("signature"))
	signer.Write([]byte("gateway-events-v2\x00" + payload))
	result := "ev2." + payload + "." + base64.RawURLEncoding.EncodeToString(signer.Sum(nil))
	if len(result) > maxEventCursorBytes {
		return "", &domain.RequestError{Code: "source_unavailable", Message: "event cursor exceeds the supported size"}
	}
	return result, nil
}

// ValidateEventPagination keeps capabilities owned by registry.Provider rather
// than duplicating them in the process configuration loader.
func (service *Service) ValidateEventPagination() error {
	if _, err := fieldmap.Default(); err != nil {
		return fmt.Errorf("field map: %w", err)
	}
	for _, source := range service.registry.Sources() {
		provider, _ := service.registry.Provider(source.Code)
		if planner, ok := provider.Events.(capability.EventQueryPlanner); ok {
			description := planner.EventQueryDescription()
			if description.CommonFilter {
				fields := make([]string, 0, len(description.Fields))
				for _, f := range description.Fields {
					fields = append(fields, f.Name)
				}
				if err := fieldmap.ValidateNativePaths(source.Code, fields); err != nil {
					return err
				}
			}
		}
		for _, name := range source.Capabilities {
			if name == domain.CapabilityEvents && len(service.eventCursorKey) != 32 {
				return fmt.Errorf("GATEWAY_CURSOR_SIGNING_KEY is required for configured event sources")
			}
		}
	}
	return nil
}
