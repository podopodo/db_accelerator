package gateway

import (
	"encoding/binary"
	"testing"

	"github.com/podopodo/db_accelerator/internal/session"
)

func TestCountPreparedParametersSkipsQuotedQuestionMarks(t *testing.T) {
	query := "SELECT ?, '?', \"?\", `?`, '\\?', ?"
	if got := countPreparedParameters(query); got != 2 {
		t.Fatalf("parameter count=%d", got)
	}
}

func TestDecodePreparedArguments(t *testing.T) {
	prepared := &preparedStatement{parameterCount: 3, parameterTypes: make([]preparedParameterType, 3), longData: make(map[uint16][]byte)}
	payload := []byte{0x17, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	payload = append(payload, 0x03, 0x00, 0xfd, 0x00, 0x08, 0x80)
	signed := int32(-7)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(signed))
	payload = append(payload, 0x02, 'o', 'k')
	payload = binary.LittleEndian.AppendUint64(payload, ^uint64(0))
	arguments, err := prepared.decodeArguments(payload)
	if err != nil {
		t.Fatal(err)
	}
	if arguments[0] != int64(-7) || string(arguments[1].([]byte)) != "ok" || arguments[2] != ^uint64(0) {
		t.Fatalf("arguments=%#v", arguments)
	}
}

func TestPreparedLongDataIsBounded(t *testing.T) {
	logical := session.NewLogical(session.Baseline{Database: "app", Charset: "utf8mb4"})
	if err := logical.AddPrepared(1); err != nil {
		t.Fatal(err)
	}
	prepared := &preparedStatement{parameterCount: 1, parameterTypes: make([]preparedParameterType, 1), longData: make(map[uint16][]byte)}
	clientSession := &clientSession{state: logical, prepared: map[uint32]*preparedStatement{1: prepared}}
	chunk := make([]byte, 1<<20)
	payload := []byte{0x18, 1, 0, 0, 0, 0, 0}
	for index := 0; index < 17; index++ {
		if err := clientSession.sendPreparedLongData(nil, append(payload, chunk...)); err != nil {
			t.Fatal(err)
		}
	}
	if prepared.longDataError == nil || len(prepared.longData[0]) > maxPreparedLongData {
		t.Fatalf("long data bytes=%d error=%v", len(prepared.longData[0]), prepared.longDataError)
	}
}

func TestPreparedHandlesNeverCrossLogicalSessionsAndDuplicateCloseIsSafe(t *testing.T) {
	firstState := session.NewLogical(session.Baseline{Database: "app"})
	secondState := session.NewLogical(session.Baseline{Database: "app"})
	_ = firstState.AddPrepared(1)
	_ = secondState.AddPrepared(1)
	first := &clientSession{state: firstState, prepared: map[uint32]*preparedStatement{1: {longData: make(map[uint16][]byte)}}}
	second := &clientSession{state: secondState, prepared: map[uint32]*preparedStatement{1: {longData: make(map[uint16][]byte)}}}
	closePacket := []byte{0x19, 1, 0, 0, 0}
	if err := first.closePrepared(closePacket); err != nil {
		t.Fatal(err)
	}
	if err := first.closePrepared(closePacket); err != nil {
		t.Fatal(err)
	}
	if second.prepared[1] == nil || len(secondState.Prepared) != 1 {
		t.Fatal("closing one logical handle affected another session")
	}
}
