package relay

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestRelayPreservesBytesAndMetrics(t *testing.T) {
	upstream := startEchoServer(t)
	server, err := New(Config{
		ListenAddress:   "127.0.0.1:0",
		UpstreamAddress: upstream.Addr().String(),
		MaxConnections:  2,
		DialTimeout:     time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}

	client, err := net.DialTimeout("tcp", server.Address(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("mysql-wire-payload")
	if _, err := client.Write(payload); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, len(payload))
	if _, err := io.ReadFull(client, received); err != nil {
		t.Fatal(err)
	}
	if string(received) != string(payload) {
		t.Fatalf("received %q want %q", received, payload)
	}
	_ = client.Close()

	deadline := time.Now().Add(time.Second)
	for server.Snapshot().Active != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	snapshot := server.Snapshot()
	if snapshot.AcceptedTotal != 1 || snapshot.ClientToDBBytes != uint64(len(payload)) || snapshot.DBToClientBytes != uint64(len(payload)) {
		t.Fatalf("unexpected metrics: %+v", snapshot)
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestRelayRejectsOverLimit(t *testing.T) {
	upstream := startHoldingServer(t)
	server, err := New(Config{
		ListenAddress:   "127.0.0.1:0",
		UpstreamAddress: upstream.Addr().String(),
		MaxConnections:  1,
		DialTimeout:     time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := net.Dial("tcp", server.Address())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	deadline := time.Now().Add(time.Second)
	for server.Snapshot().Active != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	second, err := net.Dial("tcp", server.Address())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	one := make([]byte, 1)
	if _, err := second.Read(one); err == nil {
		t.Fatal("over-limit connection stayed open")
	}
	if server.Snapshot().RejectedTotal != 1 {
		t.Fatalf("rejected = %d", server.Snapshot().RejectedTotal)
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	for _, config := range []Config{
		{ListenAddress: "bad", UpstreamAddress: "127.0.0.1:3306", MaxConnections: 1, DialTimeout: time.Second},
		{ListenAddress: "127.0.0.1:0", UpstreamAddress: "bad", MaxConnections: 1, DialTimeout: time.Second},
		{ListenAddress: "127.0.0.1:0", UpstreamAddress: "127.0.0.1:3306", MaxConnections: 0, DialTimeout: time.Second},
		{ListenAddress: "127.0.0.1:0", UpstreamAddress: "127.0.0.1:3306", MaxConnections: 1},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("accepted invalid config: %+v", config)
		}
	}
}

func startEchoServer(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, _ = io.Copy(connection, connection)
			}()
		}
	}()
	return listener
}

func startHoldingServer(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, _ = io.Copy(io.Discard, connection)
			}()
		}
	}()
	return listener
}
