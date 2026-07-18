package upstream

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

// Metadata is a non-secret snapshot of server and session identity.
type Metadata struct {
	Vendor               string `json:"vendor"`
	Version              string `json:"version"`
	VersionComment       string `json:"version_comment"`
	Database             string `json:"database"`
	CharacterSet         string `json:"character_set"`
	Collation            string `json:"collation"`
	SQLMode              string `json:"sql_mode"`
	TimeZone             string `json:"time_zone"`
	TransactionIsolation string `json:"transaction_isolation"`
	Autocommit           bool   `json:"autocommit"`
}

// Report is emitted by doctor and readiness checks.
type Report struct {
	Status    string        `json:"status"`
	Address   string        `json:"address"`
	CheckedAt time.Time     `json:"checked_at"`
	Latency   time.Duration `json:"latency_ns"`
	Metadata  Metadata      `json:"metadata"`
}

// Probe opens one physical connection, runs read-only checks, captures session
// defaults on that same connection, and then closes everything.
func (c *Connector) Probe(ctx context.Context) (Report, error) {
	ctx, cancel := boundedContext(ctx, c.healthLimit)
	defer cancel()
	started := time.Now()
	database, err := c.Open()
	if err != nil {
		return Report{}, err
	}
	defer database.Close()

	connection, err := database.Conn(ctx)
	if err != nil {
		return Report{}, classify("connect", err)
	}
	defer connection.Close()

	var health int
	if err := connection.QueryRowContext(ctx, "SELECT 1").Scan(&health); err != nil {
		return Report{}, classify("health query", err)
	}
	if health != 1 {
		return Report{}, &Error{Kind: KindServer, Operation: "health query", Err: fmt.Errorf("unexpected result %d", health)}
	}

	metadata, err := readMetadata(ctx, connection)
	if err != nil {
		return Report{}, err
	}
	return Report{
		Status:    "ok",
		Address:   c.address,
		CheckedAt: time.Now().UTC(),
		Latency:   time.Since(started),
		Metadata:  metadata,
	}, nil
}

func readMetadata(ctx context.Context, connection *sql.Conn) (Metadata, error) {
	var metadata Metadata
	var database sql.NullString
	var autocommit int
	err := connection.QueryRowContext(ctx, `SELECT VERSION(), @@version_comment, DATABASE(), @@character_set_connection, @@collation_connection, @@sql_mode, @@time_zone, @@autocommit`).Scan(
		&metadata.Version,
		&metadata.VersionComment,
		&database,
		&metadata.CharacterSet,
		&metadata.Collation,
		&metadata.SQLMode,
		&metadata.TimeZone,
		&autocommit,
	)
	if err != nil {
		return Metadata{}, classify("read server metadata", err)
	}
	metadata.Database = database.String
	metadata.Autocommit = autocommit != 0
	metadata.Vendor = detectVendor(metadata.Version, metadata.VersionComment)
	if metadata.Vendor == "unsupported" {
		return Metadata{}, &Error{Kind: KindServer, Operation: "validate server identity", Err: fmt.Errorf("unsupported server %q", metadata.VersionComment)}
	}

	err = connection.QueryRowContext(ctx, "SELECT @@transaction_isolation").Scan(&metadata.TransactionIsolation)
	if isUnknownVariable(err) {
		err = connection.QueryRowContext(ctx, "SELECT @@tx_isolation").Scan(&metadata.TransactionIsolation)
	}
	if err != nil {
		return Metadata{}, classify("read transaction isolation", err)
	}
	return metadata, nil
}

func detectVendor(version, comment string) string {
	identity := strings.ToLower(version + " " + comment)
	switch {
	case strings.Contains(identity, "mariadb"):
		return "mariadb"
	case strings.Contains(identity, "tidb") || strings.Contains(identity, "vitess"):
		return "unsupported"
	case beginsWithNumericVersion(version):
		return "mysql"
	default:
		return "unsupported"
	}
}

func beginsWithNumericVersion(version string) bool {
	if version == "" || version[0] < '0' || version[0] > '9' {
		return false
	}
	return strings.Contains(version, ".")
}

func isUnknownVariable(err error) bool {
	var mysqlError *driver.MySQLError
	return errors.As(err, &mysqlError) && mysqlError.Number == 1193
}

func boundedContext(parent context.Context, limit time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= limit {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, limit)
}
