package mysql

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ListenerConfig struct {
	MaxConnections  int
	MaxMessageBytes int
	IdleTimeout     time.Duration
}

func (c ListenerConfig) validate() error {
	if c.MaxConnections <= 0 || c.MaxMessageBytes <= 0 || c.IdleTimeout <= 0 {
		return errors.New("mysql listener limits must be positive")
	}
	return nil
}

type ConnectionInfo struct {
	ID          uint64
	RemoteAddr  string
	ConnectedAt time.Time
}

type Handler interface {
	Handle(context.Context, *Client) error
}

type HandlerFunc func(context.Context, *Client) error

func (f HandlerFunc) Handle(ctx context.Context, client *Client) error {
	return f(ctx, client)
}

type ErrorHandler func(ConnectionInfo, error)

type Server struct {
	config   ListenerConfig
	handler  Handler
	onError  ErrorHandler
	nextID   atomic.Uint64
	active   atomic.Int64
	rejected atomic.Uint64

	mu          sync.Mutex
	connections map[net.Conn]struct{}
}

func NewServer(config ListenerConfig, handler Handler, onError ErrorHandler) (*Server, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, errors.New("mysql listener handler is required")
	}
	return &Server{
		config:      config,
		handler:     handler,
		onError:     onError,
		connections: make(map[net.Conn]struct{}),
	}, nil
}

func (s *Server) ActiveConnections() int64 { return s.active.Load() }

func (s *Server) RejectedConnections() uint64 { return s.rejected.Load() }

func (s *Server) AcceptedConnections() uint64 { return s.nextID.Load() }

// Serve accepts connections from an already-bound listener. The caller owns
// address selection; Serve owns listener closure after context cancellation.
func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if listener == nil {
		return errors.New("mysql listener is required")
	}
	serveContext, cancel := context.WithCancel(ctx)
	defer cancel()

	tokens := make(chan struct{}, s.config.MaxConnections)
	var wait sync.WaitGroup
	closed := make(chan struct{})
	go func() {
		select {
		case <-serveContext.Done():
			_ = listener.Close()
			s.closeActive()
		case <-closed:
		}
	}()
	defer close(closed)
	defer func() {
		cancel()
		_ = listener.Close()
		s.closeActive()
		wait.Wait()
	}()

	backoff := time.Duration(0)
	for {
		connection, err := listener.Accept()
		if err != nil {
			if serveContext.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			if temporary, ok := err.(interface{ Temporary() bool }); ok && temporary.Temporary() {
				if backoff == 0 {
					backoff = 5 * time.Millisecond
				} else {
					backoff *= 2
				}
				if backoff > time.Second {
					backoff = time.Second
				}
				select {
				case <-time.After(backoff):
					continue
				case <-serveContext.Done():
					return nil
				}
			}
			return err
		}
		backoff = 0

		select {
		case tokens <- struct{}{}:
		case <-serveContext.Done():
			_ = connection.Close()
			return nil
		default:
			s.rejected.Add(1)
			_ = connection.Close()
			continue
		}

		info := ConnectionInfo{
			ID:          s.nextID.Add(1),
			RemoteAddr:  connection.RemoteAddr().String(),
			ConnectedAt: time.Now().UTC(),
		}
		codec, _ := NewCodec(s.config.MaxMessageBytes)
		client := &Client{
			connection:  connection,
			codec:       codec,
			idleTimeout: s.config.IdleTimeout,
			info:        info,
		}
		s.addConnection(connection)
		s.active.Add(1)
		wait.Add(1)
		go func() {
			defer wait.Done()
			defer func() { <-tokens }()
			defer s.active.Add(-1)
			defer s.removeConnection(connection)
			defer connection.Close()
			if err := s.handler.Handle(serveContext, client); err != nil && s.onError != nil {
				s.onError(info, err)
			}
		}()
	}
}

func (s *Server) addConnection(connection net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[connection] = struct{}{}
}

func (s *Server) removeConnection(connection net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.connections, connection)
}

func (s *Server) closeActive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for connection := range s.connections {
		_ = connection.Close()
	}
}

// Client owns one accepted network connection and applies idle deadlines to
// every logical message operation.
type Client struct {
	connection  net.Conn
	codec       *Codec
	idleTimeout time.Duration
	info        ConnectionInfo
	writeMu     sync.Mutex
}

func (c *Client) Info() ConnectionInfo { return c.info }

func (c *Client) ReadMessage() (Message, error) {
	if err := c.connection.SetReadDeadline(time.Now().Add(c.idleTimeout)); err != nil {
		return Message{}, err
	}
	return c.codec.ReadMessage(c.connection)
}

func (c *Client) WriteMessage(sequence uint8, payload []byte) (uint8, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.connection.SetWriteDeadline(time.Now().Add(c.idleTimeout)); err != nil {
		return sequence, err
	}
	return c.codec.WriteMessage(c.connection, sequence, payload)
}

func (c *Client) Close() error { return c.connection.Close() }
