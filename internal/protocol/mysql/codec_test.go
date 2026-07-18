package mysql

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadGoldenFrame(t *testing.T) {
	codec, err := NewCodec(1024)
	if err != nil {
		t.Fatal(err)
	}
	message, err := codec.ReadMessage(bytes.NewReader([]byte{3, 0, 0, 7, 'a', 'b', 'c'}))
	if err != nil {
		t.Fatal(err)
	}
	if message.Sequence != 7 || message.NextSequence != 8 || message.Frames != 1 || string(message.Payload) != "abc" {
		t.Fatalf("unexpected message: %+v", message)
	}
}

func TestWriteGoldenFrame(t *testing.T) {
	codec, _ := NewCodec(1024)
	var output bytes.Buffer
	next, err := codec.WriteMessage(&output, 2, []byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{3, 0, 0, 2, 'a', 'b', 'c'}
	if next != 3 || !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("next=%d output=%v want=%v", next, output.Bytes(), want)
	}
}

func TestFragmentRoundTripAndPartialReads(t *testing.T) {
	codec, err := newCodec(64, 3)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	next, err := codec.WriteMessage(&encoded, 254, []byte("abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if next != 1 {
		t.Fatalf("wrapped next sequence = %d", next)
	}
	message, err := codec.ReadMessage(&oneByteReader{reader: bytes.NewReader(encoded.Bytes())})
	if err != nil {
		t.Fatal(err)
	}
	if message.Frames != 3 || message.NextSequence != 1 || string(message.Payload) != "abcdef" {
		t.Fatalf("message = %+v", message)
	}
}

func TestReadRejectsOversizedMessageBeforePayload(t *testing.T) {
	codec, _ := newCodec(4, MaxFramePayload)
	_, err := codec.ReadMessage(bytes.NewReader([]byte{5, 0, 0, 0}))
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestReadRejectsSequenceMismatch(t *testing.T) {
	codec, _ := newCodec(64, 3)
	frames := []byte{3, 0, 0, 4, 'a', 'b', 'c', 0, 0, 0, 9}
	_, err := codec.ReadMessage(bytes.NewReader(frames))
	if !errors.Is(err, ErrSequence) {
		t.Fatalf("error = %v", err)
	}
}

func TestReadRejectsTruncatedPayload(t *testing.T) {
	codec, _ := NewCodec(64)
	_, err := codec.ReadMessage(bytes.NewReader([]byte{3, 0, 0, 0, 'a'}))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v", err)
	}
}

type oneByteReader struct{ reader io.Reader }

func (r *oneByteReader) Read(data []byte) (int, error) {
	if len(data) > 1 {
		data = data[:1]
	}
	return r.reader.Read(data)
}

func FuzzReadMessage(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{3, 0, 0, 1, 'a', 'b', 'c'})
	codec, _ := newCodec(1<<20, 1024)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = codec.ReadMessage(bytes.NewReader(data))
	})
}
