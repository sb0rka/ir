package agentaccess

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sb0rka/ir/packages/common/irconfig"
)

func repositoryFixture(t *testing.T) (*Repository, *Cipher) {
	t.Helper()
	dsn := os.Getenv("AGENT_ACCESS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AGENT_ACCESS_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "ir_agent_access_test_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); admin.Close() })
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migration, err := os.ReadFile("../../../db/migrations/investigations/012-agent-access.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, strings.ReplaceAll(string(migration), `:"DB_INV_SCHEMA_NAME"`, quoted)); err != nil {
		t.Fatal(err)
	}
	cipher, _ := NewCipher(make([]byte, 32))
	return NewRepository(pool), cipher
}

func sampleAgent(t *testing.T, cipher *Cipher, version int32) (Record, Configuration, string) {
	t.Helper()
	token, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	record := Record{ProjectID: "aabbccddee", SecretID: "00000000-0000-0000-0000-000000000001", VersionNo: version, DigestAlg: "sha256", Digest: Digest(token), Prefix: token[:14], Postfix: token[61:]}
	configuration, err := cipher.Seal(record.ProjectID, irconfig.Document{Metadata: irconfig.Metadata{Configured: true, SecretID: "00000000-0000-0000-0000-000000000002", Version: version}, Value: `[]`})
	if err != nil {
		t.Fatal(err)
	}
	return record, configuration, token
}

func TestRepositoryActivationRotationAndIsolation(t *testing.T) {
	repo, cipher := repositoryFixture(t)
	ctx := context.Background()
	first, config, firstToken := sampleAgent(t, cipher, 1)
	if _, err := repo.Verify(ctx, first.ProjectID, firstToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("missing key accepted", err)
	}
	if err := repo.SaveAgent(ctx, first, config, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Verify(ctx, first.ProjectID, firstToken); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Verify(ctx, "1122334455", firstToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("key crossed project", err)
	}
	second, config, secondToken := sampleAgent(t, cipher, 2)
	if err := repo.SaveAgent(ctx, second, config, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate create accepted", err)
	}
	if err := repo.SaveAgent(ctx, second, config, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Verify(ctx, first.ProjectID, firstToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old token survived rotation", err)
	}
	if _, err := repo.Verify(ctx, first.ProjectID, secondToken); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAgent(ctx, first, config, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("stale rotation accepted", err)
	}
	stored, err := repo.Configuration(ctx, first.ProjectID)
	if err != nil || stored.VersionNo != 2 {
		t.Fatal("configuration not activated atomically", err)
	}
	third, invalid, _ := sampleAgent(t, cipher, 3)
	invalid.SecretID = "invalid-uuid"
	if err := repo.SaveAgent(ctx, third, invalid, 2); err == nil {
		t.Fatal("invalid configuration accepted")
	}
	if _, err := repo.Verify(ctx, first.ProjectID, secondToken); err != nil {
		t.Fatal("failed activation changed active key", err)
	}
}

func TestRepositoryNeverReplacesNewerConfigurationWithOldSnapshot(t *testing.T) {
	repo, cipher := repositoryFixture(t)
	ctx := context.Background()
	first, old, _ := sampleAgent(t, cipher, 1)
	_, newer, _ := sampleAgent(t, cipher, 3)
	if err := repo.PutConfiguration(ctx, newer); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAgent(ctx, first, old, 0); err != nil {
		t.Fatal(err)
	}
	if err := repo.PutConfiguration(ctx, old); err != nil {
		t.Fatal(err)
	}
	unconfigured, err := cipher.Seal(first.ProjectID, irconfig.Document{Value: `[]`})
	if err != nil {
		t.Fatal(err)
	}
	unconfigured.UpdatedAt = newer.UpdatedAt.Add(-time.Second)
	if err := repo.PutConfiguration(ctx, unconfigured); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Configuration(ctx, first.ProjectID)
	if err != nil || stored.VersionNo != 3 || stored.SecretID != newer.SecretID {
		t.Fatal("new configuration overwritten by stale response", err)
	}

	unconfigured.UpdatedAt = newer.UpdatedAt.Add(time.Second)
	if err := repo.PutConfiguration(ctx, unconfigured); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.Configuration(ctx, first.ProjectID)
	if err != nil || stored.VersionNo != 0 || stored.SecretID != "" {
		t.Fatal("authoritative missing secret did not clear configuration", err)
	}
	if err := repo.PutConfiguration(ctx, newer); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.Configuration(ctx, first.ProjectID)
	if err != nil || stored.VersionNo != 0 || stored.SecretID != "" {
		t.Fatal("stale snapshot resurrected deleted configuration", err)
	}

	recreated := old
	recreated.SecretID = "00000000-0000-0000-0000-000000000003"
	recreated.UpdatedAt = newer.UpdatedAt.Add(2 * time.Second)
	if err := repo.PutConfiguration(ctx, recreated); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.Configuration(ctx, first.ProjectID)
	if err != nil || stored.VersionNo != 1 || stored.SecretID != recreated.SecretID {
		t.Fatal("new secret did not restore configuration", err)
	}
}

func TestRepositoryAdvisoryLockWorksWithOneConnectionPool(t *testing.T) {
	repo, cipher := repositoryFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	release, err := repo.Lock(ctx, "aabbccddee")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	first, config, _ := sampleAgent(t, cipher, 1)
	if err := repo.SaveAgent(ctx, first, config, 0); err != nil {
		t.Fatal("lock exhausted application pool", err)
	}
	blocked, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stop()
	if secondRelease, err := repo.Lock(blocked, "aabbccddee"); err == nil {
		secondRelease()
		t.Fatal("concurrent project lock was not serialized")
	}
	cancel()
	release()
	release()
	secondRelease, err := repo.Lock(context.Background(), "aabbccddee")
	if err != nil {
		t.Fatal("cancelled context prevented lock release", err)
	}
	secondRelease()
}
