package model

import "time"

type LoginSession struct {
	ID              string
	SubjectID       string
	ExternalSID     string
	RefreshHash     []byte
	EncryptedTokens []byte
	Profile         []byte
	ExpiresAt       time.Time
	Revoked         bool
	Generation      int64
}

type LoginRequest struct {
	ID               string
	Kind             string
	SessionID        string
	StateHash        []byte
	EncryptedContext []byte
	HandoffHash      []byte
	EncryptedResult  []byte
	ExpiresAt        time.Time
}

type PreviousRefresh struct {
	Session         LoginSession
	EncryptedHandle []byte
	RotatedAt       time.Time
}

type AuthRevocation struct {
	ID              string
	SessionID       string
	EncryptedTokens []byte
	Attempts        int
}
