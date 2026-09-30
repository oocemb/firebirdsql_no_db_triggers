package firebirdsql

import (
	"database/sql"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Tests for the no_db_triggers connection option, which sends
// isc_dpb_no_db_triggers on attach so database-level triggers (ON CONNECT,
// ON TRANSACTION START, ...) are not fired for the attachment.

func TestParseDSNNoDbTriggers(t *testing.T) {
	cases := []struct {
		dsn  string
		want bool
	}{
		{"user:password@localhost:3050/dbname", false},
		{"user:password@localhost:3050/dbname?no_db_triggers=false", false},
		{"user:password@localhost:3050/dbname?no_db_triggers=true", true},
		{"user:password@localhost:3050/dbname?no_db_triggers=1", true},
		{"user:password@localhost:3050/C:/db.fdb?charset=WIN1251&no_db_triggers=true", true},
		{"user:password@localhost:3050/dbname?no_db_triggers=garbage", false},
	}
	for _, c := range cases {
		dsn, err := parseDSN(c.dsn)
		require.NoError(t, err, c.dsn)
		require.Equal(t, c.want, convertToBool(dsn.options["no_db_triggers"], false), c.dsn)
	}
}

func TestAppendNoDbTriggersDPB(t *testing.T) {
	base := []byte{isc_dpb_version1}

	p := &wireProtocol{}
	require.Equal(t, base, p.appendNoDbTriggersDPB(append([]byte(nil), base...)))

	p.noDbTriggers = true
	require.Equal(t, []byte{isc_dpb_version1, isc_dpb_no_db_triggers, 1, 1}, p.appendNoDbTriggersDPB(append([]byte(nil), base...)))
}

// TestNoDbTriggersBypassesOnConnectTrigger creates a database whose ON CONNECT
// trigger rejects every attachment, then checks that a plain connection is
// refused while no_db_triggers=true gets in.
func TestNoDbTriggersBypassesOnConnectTrigger(t *testing.T) {
	dbPath, dsn, err := CreateTestDatabase("no_db_triggers_")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(dbPath) })

	setup, err := sql.Open("firebirdsql", dsn)
	require.NoError(t, err)
	_, err = setup.Exec("CREATE EXCEPTION E_REJECT_CONNECT 'connection rejected by ON CONNECT trigger'")
	require.NoError(t, err)
	_, err = setup.Exec("CREATE TRIGGER T_REJECT_CONNECT ON CONNECT AS BEGIN EXCEPTION E_REJECT_CONNECT; END")
	require.NoError(t, err)
	require.NoError(t, setup.Close())

	plain, err := sql.Open("firebirdsql", dsn)
	require.NoError(t, err)
	defer plain.Close()
	err = plain.Ping()
	require.Error(t, err, "ON CONNECT trigger should reject a plain attachment")
	require.Contains(t, err.Error(), "connection rejected by ON CONNECT trigger")

	bypass, err := sql.Open("firebirdsql", dsn+"?no_db_triggers=true")
	require.NoError(t, err)
	defer bypass.Close()
	require.NoError(t, bypass.Ping())

	var n int
	require.NoError(t, bypass.QueryRow(
		"SELECT COUNT(*) FROM RDB$TRIGGERS WHERE RDB$TRIGGER_NAME = 'T_REJECT_CONNECT'").Scan(&n))
	require.Equal(t, 1, n)

	// Drop the trigger so the database can be reused/inspected normally.
	_, err = bypass.Exec("DROP TRIGGER T_REJECT_CONNECT")
	require.NoError(t, err)
}
