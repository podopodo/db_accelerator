package gateway

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/podopodo/db_accelerator/internal/upstream"
)

func (s *Service) checkout(parent context.Context) (*sql.Conn, error) {
	var lastError error
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(parent, s.resetTimeout)
		connection, err := s.database.Conn(ctx)
		if err == nil {
			err = resetConnection(ctx, connection, s.baseline)
		}
		cancel()
		if err == nil {
			return connection, nil
		}
		lastError = err
		if connection != nil {
			discardConnection(connection)
			s.resetDiscards.Add(1)
		}
	}
	return nil, fmt.Errorf("reset upstream connection: %w", lastError)
}

func resetConnection(ctx context.Context, connection *sql.Conn, baseline upstream.Metadata) error {
	isolation, err := resetIsolation(baseline.TransactionIsolation)
	if err != nil {
		return err
	}
	if !safeResetIdentifier(baseline.CharacterSet) || !safeResetIdentifier(baseline.Collation) {
		return errors.New("unsafe character baseline returned by upstream")
	}
	commands := []string{"ROLLBACK", "SET autocommit=1"}
	if baseline.Database != "" {
		commands = append(commands, "USE "+quoteResetIdentifier(baseline.Database))
	}
	commands = append(commands,
		"SET NAMES "+baseline.CharacterSet+" COLLATE "+baseline.Collation,
		"SET SESSION time_zone = "+quoteResetString(baseline.TimeZone),
		"SET SESSION sql_mode = "+quoteResetString(baseline.SQLMode),
		"SET SESSION TRANSACTION ISOLATION LEVEL "+isolation,
		"SET SESSION TRANSACTION READ WRITE",
	)
	for index, command := range commands {
		if _, err := connection.ExecContext(ctx, command); err != nil {
			return fmt.Errorf("reset stage %d: %w", index+1, err)
		}
		// A warning belongs to the previous logical statement until another
		// statement replaces the diagnostics area. Check after the first SET,
		// not immediately after ROLLBACK, so old warnings force a discard.
		if index > 0 {
			if err := requireNoResetWarnings(ctx, connection); err != nil {
				return fmt.Errorf("reset stage %d: %w", index+1, err)
			}
		}
	}
	return verifyReset(ctx, connection, baseline)
}

func requireNoResetWarnings(ctx context.Context, connection *sql.Conn) error {
	var warnings uint64
	if err := connection.QueryRowContext(ctx, "SHOW COUNT(*) WARNINGS").Scan(&warnings); err != nil {
		return err
	}
	if warnings != 0 {
		return fmt.Errorf("reset command returned %d warnings", warnings)
	}
	return nil
}

func verifyReset(ctx context.Context, connection *sql.Conn, baseline upstream.Metadata) error {
	var database sql.NullString
	var autocommit int
	var charset, collation, sqlMode, timeZone string
	err := connection.QueryRowContext(ctx, "SELECT DATABASE(), @@autocommit, @@character_set_connection, @@collation_connection, @@sql_mode, @@time_zone").Scan(
		&database, &autocommit, &charset, &collation, &sqlMode, &timeZone,
	)
	if err != nil {
		return err
	}
	var isolation string
	err = connection.QueryRowContext(ctx, "SELECT @@transaction_isolation").Scan(&isolation)
	var mysqlError *mysqlDriver.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1193 {
		err = connection.QueryRowContext(ctx, "SELECT @@tx_isolation").Scan(&isolation)
	}
	if err != nil {
		return err
	}
	if database.String != baseline.Database || autocommit != 1 || charset != baseline.CharacterSet || collation != baseline.Collation || sqlMode != baseline.SQLMode || timeZone != baseline.TimeZone || normalizeResetIsolation(isolation) != normalizeResetIsolation(baseline.TransactionIsolation) {
		return errors.New("upstream session did not match the canonical baseline after reset")
	}
	return requireNoResetWarnings(ctx, connection)
}

func discardConnection(connection *sql.Conn) {
	_ = connection.Raw(func(any) error { return driver.ErrBadConn })
	_ = connection.Close()
}

func resetIsolation(value string) (string, error) {
	normalized := normalizeResetIsolation(value)
	switch normalized {
	case "READ UNCOMMITTED", "READ COMMITTED", "REPEATABLE READ", "SERIALIZABLE":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported transaction isolation baseline %q", value)
	}
}

func normalizeResetIsolation(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(strings.ToUpper(value), "-", " ")), " ")
}

func safeResetIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '_' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func quoteResetIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func quoteResetString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
