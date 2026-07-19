// Package relay provides the transparent, one-upstream-connection-per-client
// compatibility path used while protocol-aware pooling is under construction.
package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	ListenAddress   string
	UpstreamAddress string
	MaxConnections  int
	DialTimeout     time.Duration
}

type Snapshot struct {
	Mode              string     `json:"mode"`
	ListenAddress     string     `json:"listen_address"`
	UpstreamAddress   string     `json:"upstream_address"`
	Active            int64      `json:"active"`
	DatabaseLinks     int64      `json:"database_links"`
	IdleDatabaseLinks int64      `json:"idle_database_links"`
	WaitingWork       int64      `json:"waiting_work"`
	PinnedWork        int64      `json:"pinned_work"`
	AcceptedTotal     uint64     `json:"accepted_total"`
	RejectedTotal     uint64     `json:"rejected_total"`
	DialErrorsTotal   uint64     `json:"dial_errors_total"`
	RelayErrorsTotal  uint64     `json:"relay_errors_total"`
	ClientToDBBytes   uint64     `json:"client_to_db_bytes"`
	DBToClientBytes   uint64     `json:"db_to_client_bytes"`
	MaxConnections    int        `json:"max_connections"`
	ClientTLSMode     string     `json:"client_tls_mode"`
	ClientTLSExpires  *time.Time `json:"client_tls_expires_at,omitempty"`
}

// Server preserves the upstream wire protocol byte-for-byte. It deliberately
// does not claim pooling or acceleration; every admitted client gets one
// upstream connection.
type Server struct {
	config Config

	mu       sync.Mutex
	listener net.Listener
	conns    map[net.Conn]struct{}
	sem      chan struct{}
	wg       sync.WaitGroup
	done     chan struct{}
	stopOnce sync.Once
	errMu    sync.Mutex
	runErr   error

	active          atomic.Int64
	accepted        atomic.Uint64
	rejected        atomic.Uint64
	dialErrors      atomic.Uint64
	relayErrors     atomic.Uint64
	clientToDBBytes atomic.Uint64
	dbToClientBytes atomic.Uint64
}

func New(config Config) (*Server, error) {
	if _, _, err := net.SplitHostPort(config.ListenAddress); err != nil {
		return nil, fmt.Errorf("relay listen address: %w", err)
	}
	if _, _, err := net.SplitHostPort(config.UpstreamAddress); err != nil {
		return nil, fmt.Errorf("relay upstream address: %w", err)
	}
	if config.MaxConnections <= 0 {
		return nil, errors.New("relay max connections must be positive")
	}
	if config.DialTimeout <= 0 {
		return nil, errors.New("relay dial timeout must be positive")
	}
	return &Server{
		config: config,
		conns:  make(map[net.Conn]struct{}),
		sem:    make(chan struct{}, config.MaxConnections),
		done:   make(chan struct{}),
	}, nil
}

// Start opens the listener before returning and begins accepting in the
// background. Shutdown is driven by context cancellation or an explicit call.
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return fmt.Errorf("open relay listener: %w", err)
	}
	s.mu.Lock()
	if s.listener != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return errors.New("relay already started")
	}
	s.listener = listener
	s.mu.Unlock()

	go s.acceptLoop(ctx)
	go func() {
		select {
		case <-ctx.Done():
			s.stop()
		case <-s.done:
		}
	}()
	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer close(s.done)
	for {
		client, err := s.listener.Accept()
		if err != nil {
			if isClosedError(err) {
				return
			}
			s.setRunError(fmt.Errorf("accept relay client: %w", err))
			s.stop()
			return
		}
		select {
		case s.sem <- struct{}{}:
			s.accepted.Add(1)
			s.active.Add(1)
			s.track(client, true)
			s.wg.Add(1)
			go s.handle(ctx, client)
		default:
			s.rejected.Add(1)
			_ = client.Close()
		}
	}
}

func (s *Server) handle(ctx context.Context, client net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.track(client, false)
		_ = client.Close()
		s.active.Add(-1)
		<-s.sem
	}()

	dialer := net.Dialer{Timeout: s.config.DialTimeout, KeepAlive: 30 * time.Second}
	upstream, err := dialer.DialContext(ctx, "tcp", s.config.UpstreamAddress)
	if err != nil {
		s.dialErrors.Add(1)
		return
	}
	s.track(upstream, true)
	defer func() {
		s.track(upstream, false)
		_ = upstream.Close()
	}()

	result := make(chan error, 2)
	go copyStream(upstream, client, &s.clientToDBBytes, result)
	go copyStream(client, upstream, &s.dbToClientBytes, result)
	firstErr := <-result
	_ = client.Close()
	_ = upstream.Close()
	secondErr := <-result
	if meaningfulCopyError(firstErr) || meaningfulCopyError(secondErr) {
		s.relayErrors.Add(1)
	}
}

func copyStream(destination net.Conn, source net.Conn, counter *atomic.Uint64, result chan<- error) {
	written, err := io.CopyBuffer(destination, source, make([]byte, 32<<10))
	if written > 0 {
		counter.Add(uint64(written))
	}
	if tcp, ok := destination.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	result <- err
}

func meaningfulCopyError(err error) bool {
	return err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF)
}

func isClosedError(err error) bool {
	return errors.Is(err, net.ErrClosed)
}

func (s *Server) track(connection net.Conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if add {
		s.conns[connection] = struct{}{}
	} else {
		delete(s.conns, connection)
	}
}

func (s *Server) setRunError(err error) {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.runErr == nil {
		s.runErr = err
	}
}

func (s *Server) stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		if s.listener != nil {
			_ = s.listener.Close()
		}
		for connection := range s.conns {
			_ = connection.Close()
		}
		s.mu.Unlock()
	})
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.stop()
	waited := make(chan struct{})
	go func() {
		select {
		case <-s.done:
		default:
			<-s.done
		}
		s.wg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
		s.errMu.Lock()
		defer s.errMu.Unlock()
		return s.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return s.config.ListenAddress
	}
	return s.listener.Addr().String()
}

func (s *Server) Snapshot() Snapshot {
	return Snapshot{
		Mode:             "transparent-1-to-1",
		ListenAddress:    s.Address(),
		UpstreamAddress:  s.config.UpstreamAddress,
		Active:           s.active.Load(),
		DatabaseLinks:    s.active.Load(),
		AcceptedTotal:    s.accepted.Load(),
		RejectedTotal:    s.rejected.Load(),
		DialErrorsTotal:  s.dialErrors.Load(),
		RelayErrorsTotal: s.relayErrors.Load(),
		ClientToDBBytes:  s.clientToDBBytes.Load(),
		DBToClientBytes:  s.dbToClientBytes.Load(),
		MaxConnections:   s.config.MaxConnections,
		ClientTLSMode:    "passthrough",
	}
}
