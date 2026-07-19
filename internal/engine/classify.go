// Package engine contains conservative SQL classification used by the pooled
// protocol path. Unknown stateful behavior is refused instead of guessed.
package engine

import "strings"

type StatementKind string

const (
	StatementEmpty         StatementKind = "empty"
	StatementRead          StatementKind = "read"
	StatementWrite         StatementKind = "write"
	StatementDDL           StatementKind = "ddl"
	StatementBegin         StatementKind = "begin"
	StatementCommit        StatementKind = "commit"
	StatementRollback      StatementKind = "rollback"
	StatementSavepoint     StatementKind = "savepoint"
	StatementAutocommitOn  StatementKind = "autocommit_on"
	StatementAutocommitOff StatementKind = "autocommit_off"
	StatementSetNames      StatementKind = "set_names"
	StatementUseDatabase   StatementKind = "use_database"
	StatementWarningCount  StatementKind = "warning_count"
	StatementUnsupported   StatementKind = "unsupported"
)

type Statement struct {
	Kind   StatementKind
	SQL    string
	Reason string
}

func ClassifySQL(query string) Statement {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return Statement{Kind: StatementEmpty, SQL: query}
	}
	if hasMultipleStatements(trimmed) {
		return Statement{Kind: StatementUnsupported, SQL: query, Reason: "multiple statements are not enabled"}
	}
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	visible := stripLeadingComments(trimmed)
	if visible == "" {
		return Statement{Kind: StatementEmpty, SQL: query}
	}
	upper := strings.ToUpper(visible)
	fields := strings.Fields(upper)
	if len(fields) == 0 {
		return Statement{Kind: StatementEmpty, SQL: query}
	}

	switch fields[0] {
	case "SELECT", "SHOW", "DESC", "DESCRIBE", "EXPLAIN":
		if strings.Join(fields, " ") == "SHOW COUNT(*) WARNINGS" {
			return Statement{Kind: StatementWarningCount, SQL: trimmed}
		}
		if reason := unsafeReadReason(upper); reason != "" {
			return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: reason}
		}
		return Statement{Kind: StatementRead, SQL: trimmed}
	case "INSERT", "UPDATE", "DELETE", "REPLACE":
		if strings.Contains(upper, " RETURNING") {
			return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "DML RETURNING result sets are not supported in pooled mode"}
		}
		return Statement{Kind: StatementWrite, SQL: trimmed}
	case "CREATE", "ALTER", "DROP", "TRUNCATE", "RENAME":
		if len(fields) > 1 && fields[1] == "TEMPORARY" {
			return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "temporary objects are unsafe for a shared pool"}
		}
		return Statement{Kind: StatementDDL, SQL: trimmed}
	case "BEGIN":
		if len(fields) == 1 || len(fields) == 2 && fields[1] == "WORK" {
			return Statement{Kind: StatementBegin, SQL: trimmed}
		}
		return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "transaction modifiers are not supported in pooled mode"}
	case "START":
		if len(fields) == 2 && fields[1] == "TRANSACTION" {
			return Statement{Kind: StatementBegin, SQL: trimmed}
		}
		return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "transaction modifiers are not supported in pooled mode"}
	case "COMMIT":
		if len(fields) == 1 || len(fields) == 2 && fields[1] == "WORK" {
			return Statement{Kind: StatementCommit, SQL: trimmed}
		}
		return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "COMMIT modifiers are not supported in pooled mode"}
	case "ROLLBACK":
		if len(fields) > 1 && (fields[1] == "TO" || fields[1] == "WORK" && len(fields) > 2 && fields[2] == "TO") {
			return Statement{Kind: StatementSavepoint, SQL: trimmed}
		}
		if len(fields) == 1 || len(fields) == 2 && fields[1] == "WORK" {
			return Statement{Kind: StatementRollback, SQL: trimmed}
		}
		return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "ROLLBACK modifiers are not supported in pooled mode"}
	case "SAVEPOINT", "RELEASE":
		return Statement{Kind: StatementSavepoint, SQL: trimmed}
	case "USE":
		return Statement{Kind: StatementUseDatabase, SQL: trimmed}
	case "SET":
		compact := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(upper)
		compact = strings.TrimPrefix(compact, "SETSESSION")
		compact = strings.TrimPrefix(compact, "SET@@SESSION.")
		compact = strings.TrimPrefix(compact, "SET@@")
		compact = strings.TrimPrefix(compact, "SET")
		switch compact {
		case "AUTOCOMMIT=1", "AUTOCOMMIT=ON":
			return Statement{Kind: StatementAutocommitOn, SQL: trimmed}
		case "AUTOCOMMIT=0", "AUTOCOMMIT=OFF":
			return Statement{Kind: StatementAutocommitOff, SQL: trimmed}
		}
		if len(fields) == 3 && fields[1] == "NAMES" && strings.Trim(fields[2], "'\"") == "UTF8MB4" {
			return Statement{Kind: StatementSetNames, SQL: trimmed}
		}
		if len(fields) > 1 && fields[1] == "NAMES" {
			return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "only SET NAMES utf8mb4 is safe in pooled mode"}
		}
		return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "session-changing SET is unsafe for a shared pool"}
	case "WITH":
		return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "common table expressions are not classified in this build"}
	case "CALL", "LOAD", "LOCK", "UNLOCK", "PREPARE", "EXECUTE", "DEALLOCATE", "DO", "HANDLER", "ANALYZE", "OPTIMIZE", "CHECK", "REPAIR", "RESET", "KILL":
		return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "stateful or multi-result statement is not supported in pooled mode"}
	}
	return Statement{Kind: StatementUnsupported, SQL: trimmed, Reason: "statement type is not supported in pooled mode"}
}

func unsafeReadReason(upper string) string {
	for _, fragment := range []string{
		"GET_LOCK(", "RELEASE_LOCK(", "IS_FREE_LOCK(", "IS_USED_LOCK(",
		"LAST_INSERT_ID(", "FOUND_ROWS(", "ROW_COUNT(", "CONNECTION_ID(",
		"SQL_CALC_FOUND_ROWS", " INTO OUTFILE", " INTO DUMPFILE",
	} {
		if strings.Contains(upper, fragment) {
			return "connection-state or side-effect read is unsafe for a shared pool"
		}
	}
	withoutSystemVariables := strings.ReplaceAll(upper, "@@", "")
	if strings.Contains(withoutSystemVariables, "@") {
		return "user variables are unsafe for a shared pool"
	}
	if strings.HasPrefix(upper, "SHOW WARNINGS") || strings.HasPrefix(upper, "SHOW ERRORS") || strings.HasPrefix(upper, "SHOW STATUS") || strings.HasPrefix(upper, "SHOW SESSION") {
		return "connection-local diagnostics are unsafe for a shared pool"
	}
	return ""
}

func stripLeadingComments(query string) string {
	value := strings.TrimSpace(query)
	for value != "" {
		switch {
		case strings.HasPrefix(value, "/*"):
			end := strings.Index(value[2:], "*/")
			if end < 0 {
				return ""
			}
			value = strings.TrimSpace(value[end+4:])
		case strings.HasPrefix(value, "#"):
			end := strings.IndexByte(value, '\n')
			if end < 0 {
				return ""
			}
			value = strings.TrimSpace(value[end+1:])
		case strings.HasPrefix(value, "--") && (len(value) == 2 || value[2] == ' ' || value[2] == '\t' || value[2] == '\r' || value[2] == '\n'):
			end := strings.IndexByte(value, '\n')
			if end < 0 {
				return ""
			}
			value = strings.TrimSpace(value[end+1:])
		default:
			return value
		}
	}
	return value
}

func hasMultipleStatements(query string) bool {
	var quote byte
	escaped := false
	for index := 0; index < len(query); index++ {
		current := query[index]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' && quote != '`' {
				escaped = true
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' || current == '`' {
			quote = current
			continue
		}
		if current == ';' && stripLeadingComments(query[index+1:]) != "" {
			return true
		}
	}
	return false
}
