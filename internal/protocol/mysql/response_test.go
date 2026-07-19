package mysql

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestResponsePayloads(t *testing.T) {
	if got := hex.EncodeToString(OKPayload(1, 9, ServerStatusAutocommit, 0)); got != "00010902000000" {
		t.Fatalf("ok payload = %s", got)
	}
	if got := string(ErrorPayload(1045, "28000", "denied")[3:]); got != "#28000denied" {
		t.Fatalf("error payload = %q", got)
	}
	if got := hex.EncodeToString(EOFPayload(ServerStatusAutocommit, 2)); got != "fe02000200" {
		t.Fatalf("eof payload = %s", got)
	}
}

func TestLengthEncodedBoundaries(t *testing.T) {
	for value, expected := range map[uint64]string{
		0: "00", 250: "fa", 251: "fcfb00", 65535: "fcffff", 65536: "fd000001", 1 << 24: "fe0000000100000000",
	} {
		if got := hex.EncodeToString(AppendLengthEncodedInteger(nil, value)); got != expected {
			t.Fatalf("%d = %s want %s", value, got, expected)
		}
	}
}

func TestColumnAndRowPayload(t *testing.T) {
	column := ColumnDefinitionPayload(ColumnDefinition{Name: "answer", Type: ColumnTypeLongLong, Charset: 45, Length: 20})
	if !bytes.Contains(column, []byte("answer")) || column[len(column)-6] != ColumnTypeLongLong {
		t.Fatalf("column payload = %x", column)
	}
	row := TextRowPayload([][]byte{[]byte("42"), nil}, []bool{false, true})
	if got := hex.EncodeToString(row); got != "023432fb" {
		t.Fatalf("row payload = %s", got)
	}
}

func TestNativePassword(t *testing.T) {
	seed := []byte("01234567890123456789")
	token := NativePasswordToken("secret", seed)
	if len(token) != 20 || !NativePasswordMatches(token, "secret", seed) {
		t.Fatal("valid native password did not match")
	}
	verifier := NativePasswordVerifier("secret")
	if len(verifier) != 20 || !NativePasswordVerifierMatches(token, verifier, seed) {
		t.Fatal("valid native password verifier did not match")
	}
	if NativePasswordMatches(token, "wrong", seed) || !NativePasswordMatches(nil, "", seed) {
		t.Fatal("native password validation accepted an invalid response")
	}
	if NativePasswordVerifierMatches(token, NativePasswordVerifier("wrong"), seed) {
		t.Fatal("native password verifier accepted an invalid response")
	}
}
