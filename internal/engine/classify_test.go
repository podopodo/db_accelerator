package engine

import "testing"

func TestClassifySQL(t *testing.T) {
	for _, test := range []struct {
		query string
		kind  StatementKind
	}{
		{"SELECT 1", StatementRead},
		{"/* trace */ SHOW TABLES", StatementRead},
		{"INSERT INTO t VALUES (1)", StatementWrite},
		{"CREATE TABLE t (id INT)", StatementDDL},
		{"CREATE TEMPORARY TABLE t (id INT)", StatementUnsupported},
		{"START TRANSACTION", StatementBegin},
		{"START TRANSACTION READ ONLY", StatementUnsupported},
		{"COMMIT AND CHAIN", StatementUnsupported},
		{"ROLLBACK AND CHAIN", StatementUnsupported},
		{"ROLLBACK TO SAVEPOINT one", StatementSavepoint},
		{"SET @@session.autocommit = 0", StatementAutocommitOff},
		{"SET AUTOCOMMIT=ON", StatementAutocommitOn},
		{"SET NAMES utf8mb4", StatementSetNames},
		{"SET NAMES latin1", StatementUnsupported},
		{"SET NAMES utf8mb4 COLLATE utf8mb4_bin", StatementUnsupported},
		{"SET sql_mode='STRICT_ALL_TABLES'", StatementUnsupported},
		{"SELECT GET_LOCK('work', 1)", StatementUnsupported},
		{"SELECT LAST_INSERT_ID()", StatementUnsupported},
		{"SELECT @client_value", StatementUnsupported},
		{"SELECT @@version", StatementRead},
		{"SHOW WARNINGS", StatementUnsupported},
		{"SELECT 1 INTO OUTFILE '/tmp/x'", StatementUnsupported},
		{"INSERT INTO t VALUES (1) RETURNING id", StatementUnsupported},
		{"ANALYZE TABLE t", StatementUnsupported},
		{"SELECT ';'", StatementRead},
		{"SELECT 1; -- end", StatementRead},
		{"SELECT 1; SELECT 2", StatementUnsupported},
		{"WITH c AS (SELECT 1) SELECT * FROM c", StatementUnsupported},
	} {
		t.Run(test.query, func(t *testing.T) {
			if got := ClassifySQL(test.query); got.Kind != test.kind {
				t.Fatalf("kind = %s reason=%q want %s", got.Kind, got.Reason, test.kind)
			}
		})
	}
}
