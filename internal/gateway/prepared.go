package gateway

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/podopodo/db_accelerator/internal/engine"
	protocol "github.com/podopodo/db_accelerator/internal/protocol/mysql"
)

const maxPreparedLongData = 16 << 20

type preparedParameterType struct {
	fieldType byte
	unsigned  bool
}

type preparedStatement struct {
	query            string
	parameterCount   int
	parameterTypes   []preparedParameterType
	longData         map[uint16][]byte
	longDataBytes    int
	longDataError    error
	statement        *sql.Stmt
	transactionBound bool
}

func (s *clientSession) prepare(parent context.Context, client *protocol.Client, query string) error {
	statement := engine.ClassifySQL(query)
	if statement.Kind != engine.StatementRead && statement.Kind != engine.StatementWrite && statement.Kind != engine.StatementDDL {
		return writeError(client, 1, 1295, "HY000", "statement is not supported by the pooled prepared path")
	}
	if statement.Kind == engine.StatementDDL && s.tx != nil {
		return writeError(client, 1, 1235, "42000", "prepared DDL is refused inside a transaction")
	}
	ctx, cancel := s.preparedContext(parent, statement.Kind)
	defer cancel()
	if _, err := s.ensurePinnedConnection(ctx, "prepared_statement"); err != nil {
		return s.writeSQLError(client, err)
	}
	prepared, err := s.prepareOnCurrentConnection(ctx, statement.SQL)
	if err != nil {
		if len(s.state.Prepared) == 0 && s.tx == nil {
			s.releasePinnedConnection(true)
		}
		return s.writeSQLError(client, err)
	}
	parameterCount := countPreparedParameters(statement.SQL)
	s.nextPreparedID++
	if s.nextPreparedID == 0 {
		s.nextPreparedID++
	}
	id := s.nextPreparedID
	if s.prepared == nil {
		s.prepared = make(map[uint32]*preparedStatement)
	}
	s.prepared[id] = &preparedStatement{
		query:            statement.SQL,
		parameterCount:   parameterCount,
		parameterTypes:   make([]preparedParameterType, parameterCount),
		longData:         make(map[uint16][]byte),
		statement:        prepared,
		transactionBound: s.tx != nil,
	}
	if err := s.state.AddPrepared(id); err != nil {
		_ = prepared.Close()
		delete(s.prepared, id)
		return s.writeSQLError(client, err)
	}
	sequence, err := s.write(client, 1, protocol.PrepareOKPayload(id, 0, uint16(parameterCount), 0))
	if err != nil {
		return err
	}
	for index := 0; index < parameterCount; index++ {
		sequence, err = s.write(client, sequence, protocol.ColumnDefinitionPayload(protocol.ColumnDefinition{
			Name:    "?",
			Charset: protocol.DefaultCharsetID,
			Length:  1024,
			Type:    protocol.ColumnTypeVarString,
		}))
		if err != nil {
			return err
		}
	}
	if parameterCount > 0 {
		_, err = s.write(client, sequence, protocol.EOFPayload(s.status(), 0))
	}
	return err
}

func (s *clientSession) prepareOnCurrentConnection(ctx context.Context, query string) (*sql.Stmt, error) {
	if s.tx != nil {
		return s.tx.PrepareContext(ctx, query)
	}
	return s.pinnedConnection.PrepareContext(ctx, query)
}

func (s *clientSession) preparedContext(parent context.Context, kind engine.StatementKind) (context.Context, context.CancelFunc) {
	_, readTimeout, writeTimeout, _, _ := s.service.cfg.UpstreamDurations()
	limit := writeTimeout
	if kind == engine.StatementRead {
		limit = readTimeout
	}
	return context.WithTimeout(parent, limit)
}

func (s *clientSession) executePrepared(parent context.Context, client *protocol.Client, payload []byte) error {
	if len(payload) < 10 {
		return writeError(client, 1, 1835, "HY000", "malformed prepared execute packet")
	}
	id := binary.LittleEndian.Uint32(payload[1:5])
	prepared := s.prepared[id]
	if prepared == nil {
		return writeError(client, 1, 1243, "HY000", "unknown prepared statement handle")
	}
	if payload[5] != 0 {
		return writeError(client, 1, 1235, "42000", "prepared cursors are not supported in pooled mode")
	}
	if prepared.longDataError != nil {
		prepared.clearLongData()
		return writeError(client, 1, 1153, "08S01", "prepared long data exceeds the configured bound")
	}
	arguments, err := prepared.decodeArguments(payload)
	prepared.clearLongData()
	if err != nil {
		return writeError(client, 1, 1835, "HY000", err.Error())
	}
	classified := engine.ClassifySQL(prepared.query)
	ctx, cancel := s.preparedContext(parent, classified.Kind)
	defer cancel()
	if err := s.ensureImplicitTransaction(ctx); err != nil {
		return s.writeSQLError(client, err)
	}
	statement, temporary, err := s.statementForExecution(ctx, prepared)
	if err != nil {
		return s.writeSQLError(client, err)
	}
	if temporary {
		defer statement.Close()
	}
	if classified.Kind == engine.StatementRead {
		return s.queryPrepared(ctx, client, statement, arguments)
	}
	return s.execPrepared(ctx, client, statement, arguments)
}

func (s *clientSession) statementForExecution(ctx context.Context, prepared *preparedStatement) (*sql.Stmt, bool, error) {
	if prepared.statement == nil || prepared.transactionBound && s.tx == nil {
		if prepared.statement != nil {
			_ = prepared.statement.Close()
		}
		statement, err := s.prepareOnCurrentConnection(ctx, prepared.query)
		if err != nil {
			return nil, false, err
		}
		prepared.statement = statement
		prepared.transactionBound = s.tx != nil
	}
	if s.tx != nil && !prepared.transactionBound {
		return s.tx.StmtContext(ctx, prepared.statement), true, nil
	}
	return prepared.statement, false, nil
}

func (s *clientSession) execPrepared(ctx context.Context, client *protocol.Client, statement *sql.Stmt, arguments []any) error {
	result, err := statement.ExecContext(ctx, arguments...)
	if err != nil {
		return s.writeSQLError(client, err)
	}
	var warningSource warningCounter = s.pinnedConnection
	if s.tx != nil {
		warningSource = s.tx
	}
	warnings, err := readWarningCount(ctx, warningSource)
	if err != nil {
		return s.writeSQLError(client, err)
	}
	affected, _ := result.RowsAffected()
	insertID, _ := result.LastInsertId()
	s.state.SetResult(warnings, uint64(max(0, insertID)))
	_, err = s.write(client, 1, protocol.OKPayload(uint64(max(0, affected)), uint64(max(0, insertID)), s.status(), warnings))
	return err
}

func (s *clientSession) queryPrepared(ctx context.Context, client *protocol.Client, statement *sql.Stmt, arguments []any) error {
	rows, err := statement.QueryContext(ctx, arguments...)
	if err != nil {
		return s.writeSQLError(client, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return s.writeSQLError(client, err)
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return s.writeSQLError(client, err)
	}
	definitions := make([]protocol.ColumnDefinition, len(columns))
	sequence, err := s.write(client, 1, protocol.ColumnCountPayload(uint64(len(columns))))
	if err != nil {
		return err
	}
	for index, name := range columns {
		definitions[index] = columnDefinition(name, s.service.cfg.Upstream.Database, types[index])
		sequence, err = s.write(client, sequence, protocol.ColumnDefinitionPayload(definitions[index]))
		if err != nil {
			return err
		}
	}
	sequence, err = s.write(client, sequence, protocol.EOFPayload(s.status(), 0))
	if err != nil {
		return err
	}
	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for index := range destinations {
		destinations[index] = &values[index]
	}
	for rows.Next() {
		if err := rows.Scan(destinations...); err != nil {
			return s.writeSQLErrorAt(client, sequence, err)
		}
		payload, err := binaryPreparedRow(values, definitions, types)
		if err != nil {
			return s.writeSQLErrorAt(client, sequence, err)
		}
		sequence, err = s.write(client, sequence, payload)
		if err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return s.writeSQLErrorAt(client, sequence, err)
	}
	if err := rows.Close(); err != nil {
		return s.writeSQLErrorAt(client, sequence, err)
	}
	var warningSource warningCounter = s.pinnedConnection
	if s.tx != nil {
		warningSource = s.tx
	}
	warnings, err := readWarningCount(ctx, warningSource)
	if err != nil {
		return s.writeSQLErrorAt(client, sequence, err)
	}
	s.state.SetResult(warnings, 0)
	_, err = s.write(client, sequence, protocol.EOFPayload(s.status(), warnings))
	return err
}

func (s *clientSession) sendPreparedLongData(client *protocol.Client, payload []byte) error {
	if len(payload) < 7 {
		return writeError(client, 1, 1835, "HY000", "malformed prepared long-data packet")
	}
	id := binary.LittleEndian.Uint32(payload[1:5])
	parameter := binary.LittleEndian.Uint16(payload[5:7])
	prepared := s.prepared[id]
	if prepared == nil || int(parameter) >= prepared.parameterCount {
		return writeError(client, 1, 1243, "HY000", "unknown prepared statement handle or parameter")
	}
	if prepared.longDataBytes+len(payload[7:]) > maxPreparedLongData {
		prepared.longDataError = errors.New("prepared long data limit exceeded")
		return nil
	}
	prepared.longData[parameter] = append(prepared.longData[parameter], payload[7:]...)
	prepared.longDataBytes += len(payload[7:])
	return nil
}

func (s *clientSession) resetPrepared(client *protocol.Client, payload []byte) error {
	if len(payload) < 5 {
		return writeError(client, 1, 1835, "HY000", "malformed prepared reset packet")
	}
	prepared := s.prepared[binary.LittleEndian.Uint32(payload[1:5])]
	if prepared == nil {
		return writeError(client, 1, 1243, "HY000", "unknown prepared statement handle")
	}
	prepared.clearLongData()
	_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
	return err
}

func (s *clientSession) closePrepared(payload []byte) error {
	if len(payload) < 5 {
		return nil
	}
	id := binary.LittleEndian.Uint32(payload[1:5])
	prepared := s.prepared[id]
	if prepared == nil {
		return nil
	}
	if prepared.statement != nil {
		_ = prepared.statement.Close()
	}
	delete(s.prepared, id)
	s.state.RemovePrepared(id)
	if len(s.prepared) == 0 && s.tx == nil {
		s.releasePinnedConnection(false)
	}
	return nil
}

func (s *clientSession) closeAllPrepared() {
	for id, prepared := range s.prepared {
		if prepared.statement != nil {
			_ = prepared.statement.Close()
		}
		delete(s.prepared, id)
		s.state.RemovePrepared(id)
	}
}

func (s *clientSession) invalidateTransactionPrepared() {
	for _, prepared := range s.prepared {
		if prepared.transactionBound {
			if prepared.statement != nil {
				_ = prepared.statement.Close()
			}
			prepared.statement = nil
			prepared.transactionBound = false
		}
	}
}

func (p *preparedStatement) clearLongData() {
	clear(p.longData)
	p.longDataBytes = 0
	p.longDataError = nil
}

func (p *preparedStatement) decodeArguments(payload []byte) ([]any, error) {
	if p.parameterCount == 0 {
		return nil, nil
	}
	position := 10
	nullBytes := (p.parameterCount + 7) / 8
	if len(payload) < position+nullBytes+1 {
		return nil, errors.New("prepared execute null bitmap is truncated")
	}
	nulls := payload[position : position+nullBytes]
	position += nullBytes
	newTypes := payload[position]
	position++
	if newTypes != 0 {
		if len(payload) < position+2*p.parameterCount {
			return nil, errors.New("prepared execute type array is truncated")
		}
		for index := 0; index < p.parameterCount; index++ {
			p.parameterTypes[index] = preparedParameterType{fieldType: payload[position], unsigned: payload[position+1]&0x80 != 0}
			position += 2
		}
	} else {
		for _, parameterType := range p.parameterTypes {
			if parameterType.fieldType == 0 {
				return nil, errors.New("prepared execute omitted its first type array")
			}
		}
	}
	arguments := make([]any, p.parameterCount)
	for index := 0; index < p.parameterCount; index++ {
		if nulls[index/8]&(1<<uint(index%8)) != 0 {
			continue
		}
		if longValue, exists := p.longData[uint16(index)]; exists {
			arguments[index] = append([]byte(nil), longValue...)
			continue
		}
		value, next, err := decodePreparedParameter(payload, position, p.parameterTypes[index])
		if err != nil {
			return nil, fmt.Errorf("prepared parameter %d: %w", index, err)
		}
		arguments[index] = value
		position = next
	}
	if position != len(payload) {
		return nil, errors.New("prepared execute contains trailing value bytes")
	}
	return arguments, nil
}

func decodePreparedParameter(payload []byte, position int, parameterType preparedParameterType) (any, int, error) {
	require := func(size int) ([]byte, error) {
		if size < 0 || len(payload)-position < size {
			return nil, errors.New("value is truncated")
		}
		return payload[position : position+size], nil
	}
	switch parameterType.fieldType {
	case protocol.ColumnTypeNull:
		return nil, position, nil
	case protocol.ColumnTypeTiny:
		value, err := require(1)
		if err != nil {
			return nil, position, err
		}
		if parameterType.unsigned {
			return uint64(value[0]), position + 1, nil
		}
		return int64(int8(value[0])), position + 1, nil
	case protocol.ColumnTypeShort, protocol.ColumnTypeYear:
		value, err := require(2)
		if err != nil {
			return nil, position, err
		}
		raw := binary.LittleEndian.Uint16(value)
		if parameterType.unsigned {
			return uint64(raw), position + 2, nil
		}
		return int64(int16(raw)), position + 2, nil
	case protocol.ColumnTypeLong, protocol.ColumnTypeInt24:
		value, err := require(4)
		if err != nil {
			return nil, position, err
		}
		raw := binary.LittleEndian.Uint32(value)
		if parameterType.unsigned {
			return uint64(raw), position + 4, nil
		}
		return int64(int32(raw)), position + 4, nil
	case protocol.ColumnTypeLongLong:
		value, err := require(8)
		if err != nil {
			return nil, position, err
		}
		raw := binary.LittleEndian.Uint64(value)
		if parameterType.unsigned {
			return raw, position + 8, nil
		}
		return int64(raw), position + 8, nil
	case protocol.ColumnTypeFloat:
		value, err := require(4)
		if err != nil {
			return nil, position, err
		}
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(value))), position + 4, nil
	case protocol.ColumnTypeDouble:
		value, err := require(8)
		if err != nil {
			return nil, position, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(value)), position + 8, nil
	case protocol.ColumnTypeDate, protocol.ColumnTypeDateTime, protocol.ColumnTypeTimestamp:
		return decodePreparedDateTime(payload, position)
	case protocol.ColumnTypeTime:
		return decodePreparedTime(payload, position)
	default:
		value, next, err := readPreparedLengthEncoded(payload, position)
		if err != nil {
			return nil, position, err
		}
		return append([]byte(nil), value...), next, nil
	}
}

func decodePreparedDateTime(payload []byte, position int) (any, int, error) {
	if position >= len(payload) {
		return nil, position, errors.New("temporal length is truncated")
	}
	length := int(payload[position])
	position++
	if length == 0 {
		return []byte("0000-00-00"), position, nil
	}
	if length != 4 && length != 7 && length != 11 || len(payload)-position < length {
		return nil, position, errors.New("temporal value is malformed")
	}
	year := int(binary.LittleEndian.Uint16(payload[position : position+2]))
	month, day := time.Month(payload[position+2]), int(payload[position+3])
	hour, minute, second, microseconds := 0, 0, 0, 0
	if length >= 7 {
		hour, minute, second = int(payload[position+4]), int(payload[position+5]), int(payload[position+6])
	}
	if length == 11 {
		microseconds = int(binary.LittleEndian.Uint32(payload[position+7 : position+11]))
	}
	return time.Date(year, month, day, hour, minute, second, microseconds*1000, time.UTC), position + length, nil
}

func decodePreparedTime(payload []byte, position int) (any, int, error) {
	if position >= len(payload) {
		return nil, position, errors.New("time length is truncated")
	}
	length := int(payload[position])
	position++
	if length == 0 {
		return []byte("00:00:00"), position, nil
	}
	if length != 8 && length != 12 || len(payload)-position < length {
		return nil, position, errors.New("time value is malformed")
	}
	negative := payload[position] != 0
	days := binary.LittleEndian.Uint32(payload[position+1 : position+5])
	hours := uint64(days)*24 + uint64(payload[position+5])
	value := fmt.Sprintf("%02d:%02d:%02d", hours, payload[position+6], payload[position+7])
	if length == 12 {
		value += fmt.Sprintf(".%06d", binary.LittleEndian.Uint32(payload[position+8:position+12]))
	}
	if negative {
		value = "-" + value
	}
	return []byte(value), position + length, nil
}

func readPreparedLengthEncoded(payload []byte, position int) ([]byte, int, error) {
	if position >= len(payload) {
		return nil, position, errors.New("length-encoded value is truncated")
	}
	first := payload[position]
	position++
	var length uint64
	switch first {
	case 0xfc:
		if len(payload)-position < 2 {
			return nil, position, errors.New("length-encoded uint16 is truncated")
		}
		length = uint64(binary.LittleEndian.Uint16(payload[position : position+2]))
		position += 2
	case 0xfd:
		if len(payload)-position < 3 {
			return nil, position, errors.New("length-encoded uint24 is truncated")
		}
		length = uint64(payload[position]) | uint64(payload[position+1])<<8 | uint64(payload[position+2])<<16
		position += 3
	case 0xfe:
		if len(payload)-position < 8 {
			return nil, position, errors.New("length-encoded uint64 is truncated")
		}
		length = binary.LittleEndian.Uint64(payload[position : position+8])
		position += 8
	default:
		if first >= 0xfb {
			return nil, position, errors.New("invalid length-encoded prefix")
		}
		length = uint64(first)
	}
	if length > uint64(len(payload)-position) {
		return nil, position, errors.New("length-encoded bytes are truncated")
	}
	return payload[position : position+int(length)], position + int(length), nil
}

func binaryPreparedRow(values []any, definitions []protocol.ColumnDefinition, types []*sql.ColumnType) ([]byte, error) {
	nullBytes := (len(values) + 7 + 2) / 8
	payload := make([]byte, 1+nullBytes)
	for index, value := range values {
		if value == nil {
			bit := index + 2
			payload[1+bit/8] |= 1 << uint(bit%8)
			continue
		}
		encoded, err := encodePreparedResultValue(value, definitions[index], types[index].DatabaseTypeName())
		if err != nil {
			return nil, err
		}
		payload = append(payload, encoded...)
	}
	return payload, nil
}

func encodePreparedResultValue(value any, definition protocol.ColumnDefinition, databaseType string) ([]byte, error) {
	text, _ := textValue(value, databaseType)
	parseSigned := func(bits int) (int64, error) { return strconv.ParseInt(string(text), 10, bits) }
	parseUnsigned := func(bits int) (uint64, error) { return strconv.ParseUint(string(text), 10, bits) }
	switch definition.Type {
	case protocol.ColumnTypeTiny:
		if definition.Flags&protocol.ColumnFlagUnsigned != 0 {
			parsed, err := parseUnsigned(8)
			return []byte{byte(parsed)}, err
		}
		parsed, err := parseSigned(8)
		return []byte{byte(int8(parsed))}, err
	case protocol.ColumnTypeShort, protocol.ColumnTypeYear:
		var raw uint64
		var err error
		if definition.Flags&protocol.ColumnFlagUnsigned != 0 {
			raw, err = parseUnsigned(16)
		} else {
			var signed int64
			signed, err = parseSigned(16)
			raw = uint64(signed)
		}
		return binary.LittleEndian.AppendUint16(nil, uint16(raw)), err
	case protocol.ColumnTypeLong, protocol.ColumnTypeInt24:
		var raw uint64
		var err error
		if definition.Flags&protocol.ColumnFlagUnsigned != 0 {
			raw, err = parseUnsigned(32)
		} else {
			var signed int64
			signed, err = parseSigned(32)
			raw = uint64(signed)
		}
		return binary.LittleEndian.AppendUint32(nil, uint32(raw)), err
	case protocol.ColumnTypeLongLong:
		if definition.Flags&protocol.ColumnFlagUnsigned != 0 {
			parsed, err := parseUnsigned(64)
			return binary.LittleEndian.AppendUint64(nil, parsed), err
		}
		parsed, err := parseSigned(64)
		return binary.LittleEndian.AppendUint64(nil, uint64(parsed)), err
	case protocol.ColumnTypeFloat:
		parsed, err := strconv.ParseFloat(string(text), 32)
		return binary.LittleEndian.AppendUint32(nil, math.Float32bits(float32(parsed))), err
	case protocol.ColumnTypeDouble:
		parsed, err := strconv.ParseFloat(string(text), 64)
		return binary.LittleEndian.AppendUint64(nil, math.Float64bits(parsed)), err
	case protocol.ColumnTypeDate, protocol.ColumnTypeDateTime, protocol.ColumnTypeTimestamp:
		return encodePreparedDateTimeResult(value, definition.Type)
	case protocol.ColumnTypeTime:
		return encodePreparedTimeResult(string(text))
	default:
		return protocol.AppendLengthEncodedBytes(nil, text), nil
	}
}

func encodePreparedDateTimeResult(value any, fieldType byte) ([]byte, error) {
	var parsed time.Time
	var err error
	switch typed := value.(type) {
	case time.Time:
		parsed = typed
	default:
		text := fmt.Sprint(typed)
		if bytes, ok := value.([]byte); ok {
			text = string(bytes)
		}
		for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05.999999"} {
			parsed, err = time.ParseInLocation(layout, text, time.UTC)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if fieldType == protocol.ColumnTypeDate {
		payload := []byte{4}
		payload = binary.LittleEndian.AppendUint16(payload, uint16(parsed.Year()))
		return append(payload, byte(parsed.Month()), byte(parsed.Day())), nil
	}
	payload := []byte{7}
	payload = binary.LittleEndian.AppendUint16(payload, uint16(parsed.Year()))
	payload = append(payload, byte(parsed.Month()), byte(parsed.Day()), byte(parsed.Hour()), byte(parsed.Minute()), byte(parsed.Second()))
	if parsed.Nanosecond() != 0 {
		payload[0] = 11
		payload = binary.LittleEndian.AppendUint32(payload, uint32(parsed.Nanosecond()/1000))
	}
	return payload, nil
}

func encodePreparedTimeResult(value string) ([]byte, error) {
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	parts := strings.SplitN(value, ".", 2)
	clock := strings.Split(parts[0], ":")
	if len(clock) != 3 {
		return nil, errors.New("invalid TIME result")
	}
	hours, err := strconv.ParseUint(clock[0], 10, 32)
	if err != nil {
		return nil, err
	}
	minute, err := strconv.ParseUint(clock[1], 10, 8)
	if err != nil {
		return nil, err
	}
	second, err := strconv.ParseUint(clock[2], 10, 8)
	if err != nil {
		return nil, err
	}
	microseconds := uint64(0)
	if len(parts) == 2 {
		fraction := (parts[1] + "000000")[:6]
		microseconds, err = strconv.ParseUint(fraction, 10, 32)
		if err != nil {
			return nil, err
		}
	}
	length := byte(8)
	if microseconds != 0 {
		length = 12
	}
	payload := []byte{length, 0}
	if negative {
		payload[1] = 1
	}
	payload = binary.LittleEndian.AppendUint32(payload, uint32(hours/24))
	payload = append(payload, byte(hours%24), byte(minute), byte(second))
	if length == 12 {
		payload = binary.LittleEndian.AppendUint32(payload, uint32(microseconds))
	}
	return payload, nil
}

func countPreparedParameters(query string) int {
	count := 0
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
		if current == '?' {
			count++
		}
	}
	return count
}
