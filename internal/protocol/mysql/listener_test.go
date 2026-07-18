package mysql

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestServerEchoAndShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(ListenerConfig{
		MaxConnections:  4,
		MaxMessageBytes: 1024,
		IdleTimeout:     time.Second,
	}, HandlerFunc(func(_ context.Context, client *Client) error {
		message, readErr := client.ReadMessage()
		if readErr != nil {
			return readErr
		}
		_, writeErr := client.WriteMessage(message.NextSequence, []byte("pong:"+string(message.Payload)))
		return writeErr
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- server.Serve(ctx, listener) }()

	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	codec, _ := NewCodec(1024)
	next, err := codec.WriteMessage(connection, 0, []byte("ping"))
	if err != nil {
		t.Fatal(err)
	}
	message, err := codec.ReadMessage(connection)
	if err != nil {
		t.Fatal(err)
	}
	if message.Sequence != next || string(message.Payload) != "pong:ping" {
		t.Fatalf("response = %+v", message)
	}
	_ = connection.Close()

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestServerRejectsConnectionBeyondLimit(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	server, err := NewServer(ListenerConfig{
		MaxConnections:  1,
		MaxMessageBytes: 1024,
		IdleTimeout:     time.Second,
	}, HandlerFunc(func(ctx context.Context, _ *Client) error {
		entered <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}), nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- server.Serve(ctx, listener) }()
	first, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first connection did not enter handler")
	}

	second, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, 1)
	_, readErr := second.Read(buffer)
	_ = second.Close()
	if readErr == nil {
		t.Fatal("over-limit connection remained open")
	}

	deadline := time.Now().Add(time.Second)
	for server.RejectedConnections() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if server.RejectedConnections() != 1 {
		t.Fatalf("rejected = %d", server.RejectedConnections())
	}
	close(release)
	cancel()
	if err := <-result; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestServerAppliesIdleReadDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	handlerError := make(chan error, 1)
	server, err := NewServer(ListenerConfig{
		MaxConnections:  1,
		MaxMessageBytes: 1024,
		IdleTimeout:     40 * time.Millisecond,
	}, HandlerFunc(func(_ context.Context, client *Client) error {
		_, readErr := client.ReadMessage()
		handlerError <- readErr
		return readErr
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- server.Serve(ctx, listener) }()

	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	select {
	case err := <-handlerError:
		var networkError net.Error
		if !errors.As(err, &networkError) || !networkError.Timeout() {
			t.Fatalf("handler error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle deadline did not fire")
	}
	cancel()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
