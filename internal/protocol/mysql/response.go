package mysql

import "encoding/binary"

const (
	CommandQuit             byte = 0x01
	CommandInitDB           byte = 0x02
	CommandQuery            byte = 0x03
	CommandPing             byte = 0x0e
	CommandStmtPrepare      byte = 0x16
	CommandStmtExecute      byte = 0x17
	CommandStmtSendLongData byte = 0x18
	CommandStmtClose        byte = 0x19
	CommandStmtReset        byte = 0x1a
	CommandStmtFetch        byte = 0x1c
)

const (
	ServerStatusInTransaction uint16 = 0x0001
)

const (
	ColumnTypeDecimal    byte = 0x00
	ColumnTypeTiny       byte = 0x01
	ColumnTypeShort      byte = 0x02
	ColumnTypeLong       byte = 0x03
	ColumnTypeFloat      byte = 0x04
	ColumnTypeDouble     byte = 0x05
	ColumnTypeNull       byte = 0x06
	ColumnTypeTimestamp  byte = 0x07
	ColumnTypeLongLong   byte = 0x08
	ColumnTypeInt24      byte = 0x09
	ColumnTypeDate       byte = 0x0a
	ColumnTypeTime       byte = 0x0b
	ColumnTypeDateTime   byte = 0x0c
	ColumnTypeYear       byte = 0x0d
	ColumnTypeVarChar    byte = 0x0f
	ColumnTypeBit        byte = 0x10
	ColumnTypeJSON       byte = 0xf5
	ColumnTypeNewDecimal byte = 0xf6
	ColumnTypeEnum       byte = 0xf7
	ColumnTypeSet        byte = 0xf8
	ColumnTypeTinyBlob   byte = 0xf9
	ColumnTypeMediumBlob byte = 0xfa
	ColumnTypeLongBlob   byte = 0xfb
	ColumnTypeBlob       byte = 0xfc
	ColumnTypeVarString  byte = 0xfd
	ColumnTypeString     byte = 0xfe
	ColumnTypeGeometry   byte = 0xff
)

const (
	ColumnFlagNotNull  uint16 = 0x0001
	ColumnFlagUnsigned uint16 = 0x0020
	ColumnFlagBinary   uint16 = 0x0080
)

type ColumnDefinition struct {
	Schema        string
	Table         string
	OriginalTable string
	Name          string
	OriginalName  string
	Charset       uint16
	Length        uint32
	Type          byte
	Flags         uint16
	Decimals      byte
}

func OKPayload(affectedRows, lastInsertID uint64, status, warnings uint16) []byte {
	payload := []byte{0x00}
	payload = AppendLengthEncodedInteger(payload, affectedRows)
	payload = AppendLengthEncodedInteger(payload, lastInsertID)
	payload = binary.LittleEndian.AppendUint16(payload, status)
	payload = binary.LittleEndian.AppendUint16(payload, warnings)
	return payload
}

func ErrorPayload(code uint16, sqlState, message string) []byte {
	if len(sqlState) != 5 {
		sqlState = "HY000"
	}
	payload := []byte{0xff}
	payload = binary.LittleEndian.AppendUint16(payload, code)
	payload = append(payload, '#')
	payload = append(payload, sqlState...)
	payload = append(payload, message...)
	return payload
}

func PrepareOKPayload(statementID uint32, columns, parameters, warnings uint16) []byte {
	payload := []byte{0x00}
	payload = binary.LittleEndian.AppendUint32(payload, statementID)
	payload = binary.LittleEndian.AppendUint16(payload, columns)
	payload = binary.LittleEndian.AppendUint16(payload, parameters)
	payload = append(payload, 0x00)
	payload = binary.LittleEndian.AppendUint16(payload, warnings)
	return payload
}

func EOFPayload(status, warnings uint16) []byte {
	payload := []byte{0xfe}
	payload = binary.LittleEndian.AppendUint16(payload, warnings)
	payload = binary.LittleEndian.AppendUint16(payload, status)
	return payload
}

func ColumnCountPayload(count uint64) []byte {
	return AppendLengthEncodedInteger(nil, count)
}

func ColumnDefinitionPayload(column ColumnDefinition) []byte {
	charset := column.Charset
	if charset == 0 {
		charset = DefaultCharsetID
	}
	length := column.Length
	if length == 0 {
		length = 1024
	}
	payload := AppendLengthEncodedBytes(nil, []byte("def"))
	payload = AppendLengthEncodedBytes(payload, []byte(column.Schema))
	payload = AppendLengthEncodedBytes(payload, []byte(column.Table))
	payload = AppendLengthEncodedBytes(payload, []byte(column.OriginalTable))
	payload = AppendLengthEncodedBytes(payload, []byte(column.Name))
	payload = AppendLengthEncodedBytes(payload, []byte(column.OriginalName))
	payload = append(payload, 0x0c)
	payload = binary.LittleEndian.AppendUint16(payload, charset)
	payload = binary.LittleEndian.AppendUint32(payload, length)
	payload = append(payload, column.Type)
	payload = binary.LittleEndian.AppendUint16(payload, column.Flags)
	payload = append(payload, column.Decimals, 0x00, 0x00)
	return payload
}

func TextRowPayload(values [][]byte, nulls []bool) []byte {
	payload := make([]byte, 0, len(values)*8)
	for index, value := range values {
		if index < len(nulls) && nulls[index] {
			payload = append(payload, 0xfb)
			continue
		}
		payload = AppendLengthEncodedBytes(payload, value)
	}
	return payload
}

func AppendLengthEncodedBytes(target, value []byte) []byte {
	target = AppendLengthEncodedInteger(target, uint64(len(value)))
	return append(target, value...)
}

func AppendLengthEncodedInteger(target []byte, value uint64) []byte {
	switch {
	case value < 0xfb:
		return append(target, byte(value))
	case value <= 0xffff:
		target = append(target, 0xfc)
		return binary.LittleEndian.AppendUint16(target, uint16(value))
	case value <= 0xffffff:
		return append(target, 0xfd, byte(value), byte(value>>8), byte(value>>16))
	default:
		target = append(target, 0xfe)
		return binary.LittleEndian.AppendUint64(target, value)
	}
}
