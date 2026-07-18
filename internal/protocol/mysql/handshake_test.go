package mysql

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"
)

func TestGreetingPayload(t *testing.T) {
	config := DefaultHandshakeConfig(42)
	handshake, err := NewHandshake(config)
	if err != nil {
		t.Fatal(err)
	}
	payload := handshake.GreetingPayload()
	if payload[0] != ProtocolVersion10 {
		t.Fatalf("protocol = %d", payload[0])
	}
	if !bytes.Contains(payload, []byte(config.ServerVersion+"\x00")) {
		t.Fatal("server version missing")
	}
	if binary.LittleEndian.Uint32(payload[len(config.ServerVersion)+2:]) != 42 {
		t.Fatal("connection ID missing")
	}
	if bytes.Count(handshake.Seed(), []byte{0}) != 0 {
		t.Fatal("seed contains null byte")
	}
}

func TestHandshakeGreetingAndResponseExchange(t *testing.T) {
	serverConnection, clientConnection := net.Pipe()
	defer serverConnection.Close()
	defer clientConnection.Close()
	codec, _ := NewCodec(1 << 20)
	serverClient := &Client{
		connection:  serverConnection,
		codec:       codec,
		idleTimeout: time.Second,
	}
	handshake, _ := NewHandshake(DefaultHandshakeConfig(9))

	serverResult := make(chan error, 1)
	go func() {
		if err := handshake.SendGreeting(serverClient); err != nil {
			serverResult <- err
			return
		}
		response, err := handshake.ReadResponse(serverClient)
		if err == nil && response.Username != "driver-user" {
			err = errors.New("wrong handshake username")
		}
		serverResult <- err
	}()

	greeting, err := codec.ReadMessage(clientConnection)
	if err != nil {
		t.Fatal(err)
	}
	if greeting.Sequence != 0 || greeting.Payload[0] != ProtocolVersion10 {
		t.Fatalf("greeting = %+v", greeting)
	}
	response := buildHandshakeResponse(DefaultServerCapabilities, 45, "driver-user", []byte{1}, "", DefaultAuthPlugin, nil)
	if _, err := codec.WriteMessage(clientConnection, greeting.NextSequence, response); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-context.Background().Done():
		t.Fatal("unreachable context completed")
	case <-time.After(time.Second):
		t.Fatal("handshake exchange timed out")
	}
}

func TestParseHandshakeResponse(t *testing.T) {
	config := DefaultHandshakeConfig(1)
	handshake, _ := NewHandshake(config)
	capabilities := DefaultServerCapabilities | ClientFoundRows
	payload := buildHandshakeResponse(capabilities, 45, "app_user", []byte{1, 2, 3}, "app", DefaultAuthPlugin, map[string]string{
		"_client_name": "go-sql-driver",
		"secret":       "must-not-log",
	})
	response, err := handshake.ParseResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if response.Username != "app_user" || response.Database != "app" || response.AuthPlugin != DefaultAuthPlugin {
		t.Fatalf("response = %+v", response)
	}
	if !bytes.Equal(response.AuthResponse, []byte{1, 2, 3}) {
		t.Fatalf("auth response = %v", response.AuthResponse)
	}
	safe := SafeConnectionAttributes(response.Attributes)
	if safe["_client_name"] != "go-sql-driver" {
		t.Fatalf("safe attributes = %v", safe)
	}
	if _, ok := safe["secret"]; ok {
		t.Fatal("unapproved attribute was retained")
	}
}

func TestParseTLSRequest(t *testing.T) {
	config := DefaultHandshakeConfig(1)
	config.Capabilities |= ClientSSL
	handshake, _ := NewHandshake(config)
	payload := make([]byte, 32)
	binary.LittleEndian.PutUint32(payload, uint32(DefaultServerCapabilities|ClientSSL))
	payload[8] = 45
	response, err := handshake.ParseResponse(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !response.WantsTLS {
		t.Fatal("TLS request was not recognized")
	}
}

func TestParseRejectsUnsupportedCompression(t *testing.T) {
	handshake, _ := NewHandshake(DefaultHandshakeConfig(1))
	payload := buildHandshakeResponse(DefaultServerCapabilities|ClientCompress, 45, "user", nil, "", DefaultAuthPlugin, nil)
	_, err := handshake.ParseResponse(payload)
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsUnknownCapability(t *testing.T) {
	handshake, _ := NewHandshake(DefaultHandshakeConfig(1))
	payload := buildHandshakeResponse(DefaultServerCapabilities|Capability(1<<30), 45, "user", nil, "", DefaultAuthPlugin, nil)
	_, err := handshake.ParseResponse(payload)
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsUnsupportedCharsetAndMalformedPayload(t *testing.T) {
	handshake, _ := NewHandshake(DefaultHandshakeConfig(1))
	payload := buildHandshakeResponse(DefaultServerCapabilities, 1, "user", nil, "", DefaultAuthPlugin, nil)
	if _, err := handshake.ParseResponse(payload); !errors.Is(err, ErrUnsupportedCharset) {
		t.Fatalf("charset error = %v", err)
	}
	if _, err := handshake.ParseResponse([]byte{1, 2, 3}); !errors.Is(err, ErrMalformedHandshake) {
		t.Fatalf("malformed error = %v", err)
	}
}

func TestParseRejectsOversizedAttributes(t *testing.T) {
	config := DefaultHandshakeConfig(1)
	config.MaxAttributesBytes = 4
	handshake, _ := NewHandshake(config)
	payload := buildHandshakeResponse(DefaultServerCapabilities, 45, "user", nil, "", DefaultAuthPlugin, map[string]string{"key": "value"})
	if _, err := handshake.ParseResponse(payload); !errors.Is(err, ErrMalformedHandshake) {
		t.Fatalf("error = %v", err)
	}
}

func FuzzParseHandshakeResponse(f *testing.F) {
	handshake, _ := NewHandshake(DefaultHandshakeConfig(1))
	f.Add([]byte{1, 2, 3})
	f.Add(buildHandshakeResponse(DefaultServerCapabilities, 45, "user", []byte{1}, "", DefaultAuthPlugin, nil))
	f.Fuzz(func(t *testing.T, payload []byte) {
		_, _ = handshake.ParseResponse(payload)
	})
}

func buildHandshakeResponse(capabilities Capability, charset byte, username string, auth []byte, database, plugin string, attributes map[string]string) []byte {
	payload := make([]byte, 32)
	binary.LittleEndian.PutUint32(payload, uint32(capabilities))
	binary.LittleEndian.PutUint32(payload[4:], 1<<20)
	payload[8] = charset
	payload = appendNullTerminated(payload, username)
	payload = appendLenEncoded(payload, auth)
	if capabilities&ClientConnectWithDB != 0 {
		payload = appendNullTerminated(payload, database)
	}
	if capabilities&ClientPluginAuth != 0 {
		payload = appendNullTerminated(payload, plugin)
	}
	if capabilities&ClientConnectAttrs != 0 {
		var encoded []byte
		for key, value := range attributes {
			encoded = appendLenEncoded(encoded, []byte(key))
			encoded = appendLenEncoded(encoded, []byte(value))
		}
		payload = appendLenEncoded(payload, encoded)
	}
	return payload
}

func appendLenEncoded(target, value []byte) []byte {
	if len(value) >= 0xfb {
		panic("test helper only supports short values")
	}
	target = append(target, byte(len(value)))
	return append(target, value...)
}
