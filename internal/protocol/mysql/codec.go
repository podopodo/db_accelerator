// Package mysql implements the client-facing MySQL wire protocol.
package mysql

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	HeaderBytes      = 4
	MaxFramePayload  = (1 << 24) - 1
	DefaultMaxPacket = 64 << 20
)

var (
	ErrMessageTooLarge = errors.New("mysql message exceeds configured limit")
	ErrSequence        = errors.New("mysql packet sequence mismatch")
	ErrInvalidCodec    = errors.New("invalid mysql codec limits")
)

// Message is one logical MySQL message assembled from one or more frames.
type Message struct {
	Sequence     uint8
	NextSequence uint8
	Frames       int
	Payload      []byte
}

// Codec bounds logical message size and handles MySQL frame fragmentation.
type Codec struct {
	maxMessageBytes int
	framePayload    int
}

func NewCodec(maxMessageBytes int) (*Codec, error) {
	return newCodec(maxMessageBytes, MaxFramePayload)
}

func newCodec(maxMessageBytes, framePayload int) (*Codec, error) {
	if maxMessageBytes <= 0 || framePayload <= 0 || framePayload > MaxFramePayload {
		return nil, ErrInvalidCodec
	}
	return &Codec{maxMessageBytes: maxMessageBytes, framePayload: framePayload}, nil
}

func (c *Codec) ReadMessage(reader io.Reader) (Message, error) {
	var message Message
	var expected uint8
	for {
		var header [HeaderBytes]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return Message{}, err
		}
		length := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
		sequence := header[3]
		if message.Frames == 0 {
			message.Sequence = sequence
			expected = sequence
		}
		if sequence != expected {
			return Message{}, fmt.Errorf("%w: got %d want %d", ErrSequence, sequence, expected)
		}
		if length > c.maxMessageBytes-len(message.Payload) {
			return Message{}, ErrMessageTooLarge
		}

		start := len(message.Payload)
		message.Payload = append(message.Payload, make([]byte, length)...)
		if _, err := io.ReadFull(reader, message.Payload[start:]); err != nil {
			return Message{}, err
		}
		message.Frames++
		expected++
		if length < c.framePayload {
			message.NextSequence = expected
			return message, nil
		}
	}
}

// WriteMessage writes one logical message and returns the next packet sequence.
func (c *Codec) WriteMessage(writer io.Writer, sequence uint8, payload []byte) (uint8, error) {
	if len(payload) > c.maxMessageBytes {
		return sequence, ErrMessageTooLarge
	}
	remaining := payload
	for len(remaining) >= c.framePayload {
		if err := writeFrame(writer, sequence, remaining[:c.framePayload]); err != nil {
			return sequence, err
		}
		sequence++
		remaining = remaining[c.framePayload:]
	}
	if err := writeFrame(writer, sequence, remaining); err != nil {
		return sequence, err
	}
	return sequence + 1, nil
}

func writeFrame(writer io.Writer, sequence uint8, payload []byte) error {
	if len(payload) > MaxFramePayload {
		return ErrMessageTooLarge
	}
	var header [HeaderBytes]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(payload)))
	header[3] = sequence
	if err := writeAll(writer, header[:]); err != nil {
		return err
	}
	return writeAll(writer, payload)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
