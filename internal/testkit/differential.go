package testkit

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

const (
	differentialStatusInTransaction uint16 = 0x0001
	differentialStatusAutocommit    uint16 = 0x0002
)

type DifferentialColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	HasNull    bool   `json:"has_nullability"`
	Precision  int64  `json:"precision"`
	Scale      int64  `json:"scale"`
	HasDecimal bool   `json:"has_decimal"`
}

type DifferentialCell struct {
	Null   bool   `json:"null"`
	Base64 string `json:"base64,omitempty"`
}

type DifferentialError struct {
	Code     uint16 `json:"code"`
	SQLState string `json:"sql_state"`
	Message  string `json:"message"`
}

type DifferentialSnapshot struct {
	Columns      []DifferentialColumn `json:"columns,omitempty"`
	Rows         [][]DifferentialCell `json:"rows,omitempty"`
	Status       uint16               `json:"status"`
	Warnings     uint16               `json:"warnings"`
	AffectedRows int64                `json:"affected_rows"`
	InsertID     int64                `json:"insert_id"`
	Error        *DifferentialError   `json:"error,omitempty"`
}

type DifferentialMismatch struct {
	Field  string `json:"field"`
	Direct any    `json:"direct"`
	Proxy  any    `json:"proxy"`
}

type DifferentialReproduction struct {
	SchemaVersion int                    `json:"schema_version"`
	CreatedAt     time.Time              `json:"created_at"`
	Seed          int64                  `json:"seed"`
	Server        string                 `json:"server"`
	Driver        string                 `json:"driver"`
	Operation     string                 `json:"operation"`
	Direct        DifferentialSnapshot   `json:"direct"`
	Proxy         DifferentialSnapshot   `json:"proxy"`
	Mismatches    []DifferentialMismatch `json:"mismatches"`
}

func CaptureQuery(ctx context.Context, database *sql.DB, statement string) DifferentialSnapshot {
	connection, err := database.Conn(ctx)
	if err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	defer connection.Close()
	rows, err := connection.QueryContext(ctx, statement)
	if err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		rows.Close()
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	snapshot := DifferentialSnapshot{Columns: make([]DifferentialColumn, len(types))}
	for index, column := range types {
		snapshot.Columns[index].Name = column.Name()
		snapshot.Columns[index].Type = column.DatabaseTypeName()
		snapshot.Columns[index].Nullable, snapshot.Columns[index].HasNull = column.Nullable()
		snapshot.Columns[index].Precision, snapshot.Columns[index].Scale, snapshot.Columns[index].HasDecimal = column.DecimalSize()
	}
	values := make([]any, len(types))
	destinations := make([]any, len(types))
	for index := range destinations {
		destinations[index] = &values[index]
	}
	for rows.Next() {
		if err := rows.Scan(destinations...); err != nil {
			rows.Close()
			return DifferentialSnapshot{Error: differentialError(err)}
		}
		row := make([]DifferentialCell, len(values))
		for index, value := range values {
			row[index] = differentialCell(value)
		}
		snapshot.Rows = append(snapshot.Rows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	if err := rows.Close(); err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	if err := connection.QueryRowContext(ctx, "SHOW COUNT(*) WARNINGS").Scan(&snapshot.Warnings); err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	if err := captureDifferentialStatus(ctx, connection, &snapshot); err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	return snapshot
}

func CaptureExec(ctx context.Context, database *sql.DB, statement string) DifferentialSnapshot {
	connection, err := database.Conn(ctx)
	if err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	defer connection.Close()
	result, err := connection.ExecContext(ctx, statement)
	if err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	snapshot := DifferentialSnapshot{}
	snapshot.AffectedRows, _ = result.RowsAffected()
	snapshot.InsertID, _ = result.LastInsertId()
	if err := connection.QueryRowContext(ctx, "SHOW COUNT(*) WARNINGS").Scan(&snapshot.Warnings); err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	if err := captureDifferentialStatus(ctx, connection, &snapshot); err != nil {
		return DifferentialSnapshot{Error: differentialError(err)}
	}
	return snapshot
}

func captureDifferentialStatus(ctx context.Context, connection *sql.Conn, snapshot *DifferentialSnapshot) error {
	var autocommit, inTransaction uint8
	if err := connection.QueryRowContext(ctx, "SELECT @@session.autocommit, @@session.in_transaction").Scan(&autocommit, &inTransaction); err != nil {
		return err
	}
	if autocommit != 0 {
		snapshot.Status |= differentialStatusAutocommit
	}
	if inTransaction != 0 {
		snapshot.Status |= differentialStatusInTransaction
	}
	return nil
}

func CompareDifferential(direct, proxy DifferentialSnapshot) []DifferentialMismatch {
	var mismatches []DifferentialMismatch
	compare := func(field string, first, second any) {
		if !reflect.DeepEqual(first, second) {
			mismatches = append(mismatches, DifferentialMismatch{Field: field, Direct: first, Proxy: second})
		}
	}
	compare("columns", direct.Columns, proxy.Columns)
	compare("rows", direct.Rows, proxy.Rows)
	compare("status", direct.Status, proxy.Status)
	compare("warnings", direct.Warnings, proxy.Warnings)
	compare("affected_rows", direct.AffectedRows, proxy.AffectedRows)
	compare("insert_id", direct.InsertID, proxy.InsertID)
	compare("error", direct.Error, proxy.Error)
	return mismatches
}

func SaveDifferentialReproduction(path string, reproduction DifferentialReproduction) error {
	if len(reproduction.Mismatches) == 0 {
		return errors.New("differential reproduction requires at least one mismatch")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create differential output directory: %w", err)
	}
	encoded, err := json.MarshalIndent(reproduction, "", "  ")
	if err != nil {
		return fmt.Errorf("encode differential reproduction: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write differential reproduction: %w", err)
	}
	return nil
}

func differentialCell(value any) DifferentialCell {
	if value == nil {
		return DifferentialCell{Null: true}
	}
	var encoded []byte
	switch typed := value.(type) {
	case []byte:
		encoded = append([]byte(nil), typed...)
	case string:
		encoded = []byte(typed)
	case time.Time:
		encoded = []byte(typed.UTC().Format(time.RFC3339Nano))
	default:
		encoded = []byte(fmt.Sprint(typed))
	}
	return DifferentialCell{Base64: base64.StdEncoding.EncodeToString(encoded)}
}

func differentialError(err error) *DifferentialError {
	if err == nil {
		return nil
	}
	var mysqlError *driver.MySQLError
	if errors.As(err, &mysqlError) {
		return &DifferentialError{Code: mysqlError.Number, SQLState: string(mysqlError.SQLState[:]), Message: mysqlError.Message}
	}
	return &DifferentialError{Message: err.Error()}
}
