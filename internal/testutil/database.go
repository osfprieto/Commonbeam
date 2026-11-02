package testutil

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/osfprieto/Commonbeam/internal/protocol"
	"github.com/osfprieto/Commonbeam/internal/store"
)

func Database(t *testing.T) (*store.Store, string, []byte) {
	t.Helper()
	dsn := os.Getenv("COMMONBEAM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("real PostgreSQL not configured; database behavior NOT validated")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	schema := "cb_" + protocol.RandomID()[:16]
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close(ctx)
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("test database URL must be a PostgreSQL URL")
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	isolated := u.String()
	key := make([]byte, 32)
	copy(key, protocol.RandomID())
	db, err := store.Open(ctx, isolated, "test-network", key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db, isolated, key
}
