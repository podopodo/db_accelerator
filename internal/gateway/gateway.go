// Package gateway implements the experimental protocol-aware pooled MySQL
// path. It terminates a conservative subset of the MySQL text protocol so
// idle logical clients do not consume physical database connections.
package gateway

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/engine"
	protocol "github.com/podopodo/db_accelerator/internal/protocol/mysql"
	"github.com/podopodo/db_accelerator/internal/relay"
	"github.com/podopodo/db_accelerator/internal/upstream"
)

const maxErrorMessageBytes = 2048

var errQueueFull = errors.New("accelerator request queue is full")

type Service struct {
	cfg               config.Config
	database          *sql.DB
	logger            *slog.Logger
	server            *protocol.Server
	auth              *authLimiter
	clientTLS         *tls.Config
	clientCertificate *clientCertificateStore
	clientVerifier    []byte
	permission        permissionIdentity
	admission         chan struct{}
	queue             chan struct{}

	mu       sync.Mutex
	listener net.Listener
	cancel   context.CancelFunc
	done     chan error

	waiting       atomic.Int64
	queuedBytes   atomic.Int64
	pinned        atomic.Int64
	queryErrors   atomic.Uint64
	clientBytes   atomic.Uint64
	databaseBytes atomic.Uint64
}

func New(cfg config.Config, secrets config.Secrets, connector *upstream.Connector, logger *slog.Logger) (*Service, error) {
	if connector == nil {
		return nil, errors.New("pooled gateway requires an upstream connector")
	}
	clientTLS, clientCertificate, err := newClientTLS(cfg.Server)
	if err != nil {
		return nil, err
	}
	database, err := connector.OpenPool(cfg.Limits.MaxUpstreamConnections)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	service := &Service{
		cfg:               cfg,
		database:          database,
		logger:            logger,
		auth:              newAuthLimiter(),
		clientTLS:         clientTLS,
		clientCertificate: clientCertificate,
		clientVerifier:    protocol.NativePasswordVerifier(secrets.ClientPassword.Reveal()),
		permission:        newPermissionIdentity(cfg),
		admission:         make(chan struct{}, cfg.Limits.MaxUpstreamConnections),
		queue:             make(chan struct{}, cfg.Limits.MaxQueuedRequests),
		done:              make(chan error, 1),
	}
	server, err := protocol.NewServer(protocol.ListenerConfig{
		MaxConnections:  cfg.Limits.MaxLogicalConnections,
		MaxMessageBytes: protocol.DefaultMaxPacket,
		IdleTimeout:     5 * time.Minute,
	}, protocol.HandlerFunc(service.handleClient), service.handleConnectionError)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	service.server = server
	return service, nil
}

func (s *Service) Start(parent context.Context) error {
	listener, err := net.Listen("tcp", s.cfg.Server.MySQLListen)
	if err != nil {
		return fmt.Errorf("open pooled mysql listener: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	if s.listener != nil {
		s.mu.Unlock()
		cancel()
		_ = listener.Close()
		return errors.New("pooled mysql listener already started")
	}
	s.listener = listener
	s.cancel = cancel
	s.mu.Unlock()
	go func() { s.done <- s.server.Serve(ctx, listener) }()
	return nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	listener := s.listener
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if listener != nil {
		_ = listener.Close()
	}
	select {
	case err := <-s.done:
		closeErr := s.database.Close()
		if err != nil {
			return err
		}
		return closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return s.cfg.Server.MySQLListen
	}
	return s.listener.Addr().String()
}

func (s *Service) Snapshot() relay.Snapshot {
	stats := s.database.Stats()
	return relay.Snapshot{
		Mode:              "protocol-pooled",
		ListenAddress:     s.Address(),
		UpstreamAddress:   net.JoinHostPort(s.cfg.Upstream.Host, strconv.Itoa(s.cfg.Upstream.Port)),
		Active:            s.server.ActiveConnections(),
		DatabaseLinks:     int64(stats.InUse),
		IdleDatabaseLinks: int64(stats.Idle),
		WaitingWork:       s.waiting.Load(),
		PinnedWork:        s.pinned.Load(),
		AcceptedTotal:     s.server.AcceptedConnections(),
		RejectedTotal:     s.server.RejectedConnections(),
		RelayErrorsTotal:  s.queryErrors.Load(),
		ClientToDBBytes:   s.clientBytes.Load(),
		DBToClientBytes:   s.databaseBytes.Load(),
		MaxConnections:    s.cfg.Limits.MaxUpstreamConnections,
		ClientTLSMode:     s.cfg.Server.MySQLTLSMode,
		ClientTLSExpires:  s.clientCertificate.expiry(),
	}
}

func (s *Service) handleConnectionError(info protocol.ConnectionInfo, err error) {
	if errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return
	}
	s.logger.Warn("pooled mysql client ended", "connection_id", info.ID, "remote", info.RemoteAddr, "error", err)
}

func (s *Service) handleClient(ctx context.Context, client *protocol.Client) error {
	handshakeConfig := protocol.DefaultHandshakeConfig(uint32(client.Info().ID))
	handshakeConfig.ServerVersion = "DatabaseAccelerator-" + buildinfo.Version
	handshakeConfig.AuthPlugin = protocol.NativePasswordPlugin
	handshakeConfig.Capabilities &^= protocol.ClientDeprecateEOF |
		protocol.ClientSessionTrack |
		protocol.ClientMultiResults |
		protocol.ClientPSMultiResults
	if s.clientTLS != nil {
		handshakeConfig.Capabilities |= protocol.ClientSSL
	} else {
		handshakeConfig.Capabilities &^= protocol.ClientSSL
	}
	handshake, err := protocol.NewHandshake(handshakeConfig)
	if err != nil {
		return err
	}
	if err := handshake.SendGreeting(client); err != nil {
		return err
	}
	response, err := handshake.ReadResponse(client)
	if err != nil {
		_ = writeError(client, 2, 1043, "08S01", "invalid handshake response")
		return err
	}
	remoteAddress := client.Info().RemoteAddr
	authSequence := uint8(2)
	if response.WantsTLS {
		if s.clientTLS == nil {
			return errors.New("client requested unavailable TLS")
		}
		if err := client.UpgradeTLS(ctx, s.clientTLS); err != nil {
			return fmt.Errorf("upgrade client TLS: %w", err)
		}
		response, err = handshake.ReadResponseSequence(client, 2)
		if err != nil {
			return err
		}
		authSequence = 3
	} else if s.clientTLS != nil {
		_ = writeError(client, authSequence, 3159, "HY000", "secure transport required by Database Accelerator")
		return errors.New("client TLS is required")
	}
	if !s.auth.allowed(remoteAddress) {
		_ = writeError(client, authSequence, 1045, "28000", "access denied by Database Accelerator")
		return errors.New("client authentication temporarily locked")
	}
	if err := s.authenticate(response, handshake.Seed()); err != nil {
		s.auth.failure(remoteAddress)
		_ = writeError(client, authSequence, 1045, "28000", "access denied by Database Accelerator")
		return err
	}
	s.auth.success(remoteAddress)
	if _, err := client.WriteMessage(authSequence, protocol.OKPayload(0, 0, protocol.ServerStatusAutocommit, 0)); err != nil {
		return err
	}

	session := &clientSession{service: s, context: ctx, autocommit: true}
	defer session.close()
	for {
		message, err := client.ReadMessage()
		if err != nil {
			return err
		}
		if message.Sequence != 0 || len(message.Payload) == 0 {
			return protocol.ErrSequence
		}
		s.clientBytes.Add(uint64(len(message.Payload) + protocol.HeaderBytes))
		command := message.Payload[0]
		switch command {
		case protocol.CommandQuit:
			return nil
		case protocol.CommandPing:
			_, err = session.write(client, 1, protocol.OKPayload(0, 0, session.status(), 0))
		case protocol.CommandInitDB:
			err = session.initDatabase(client, string(message.Payload[1:]))
		case protocol.CommandQuery:
			err = session.query(ctx, client, string(message.Payload[1:]))
		default:
			err = writeError(client, 1, 1047, "08S01", "command is not supported in pooled mode")
		}
		if err != nil {
			return err
		}
	}
}

func (s *Service) authenticate(response protocol.HandshakeResponse, seed []byte) error {
	if response.Username != s.permission.ClientUser {
		return errors.New("client user does not match configured accelerator identity")
	}
	if response.AuthPlugin != protocol.NativePasswordPlugin {
		return errors.New("unsupported client authentication plugin")
	}
	if response.Database != "" && response.Database != s.cfg.Upstream.Database {
		return errors.New("client database does not match configured upstream database")
	}
	if !protocol.NativePasswordVerifierMatches(response.AuthResponse, s.clientVerifier, seed) {
		return errors.New("client password did not match configured accelerator identity")
	}
	return nil
}

func (s *Service) acquire(ctx context.Context, requestBytes int64) (func(), error) {
	select {
	case s.admission <- struct{}{}:
		return func() { <-s.admission }, nil
	default:
	}
	if requestBytes < 0 {
		requestBytes = 0
	}
	if requestBytes > s.cfg.Limits.MaxQueuedBytes {
		return nil, errQueueFull
	}
	select {
	case s.queue <- struct{}{}:
	default:
		return nil, errQueueFull
	}
	if requestBytes > 0 {
		queued := s.queuedBytes.Add(requestBytes)
		if queued > s.cfg.Limits.MaxQueuedBytes {
			s.queuedBytes.Add(-requestBytes)
			<-s.queue
			return nil, errQueueFull
		}
	}
	s.waiting.Add(1)
	defer func() {
		s.waiting.Add(-1)
		s.queuedBytes.Add(-requestBytes)
		<-s.queue
	}()
	select {
	case s.admission <- struct{}{}:
		return func() { <-s.admission }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type clientSession struct {
	service    *Service
	context    context.Context
	tx         *sql.Tx
	txCancel   context.CancelFunc
	release    func()
	autocommit bool
}

func (s *clientSession) status() uint16 {
	var status uint16
	if s.autocommit {
		status |= protocol.ServerStatusAutocommit
	}
	if s.tx != nil {
		status |= protocol.ServerStatusInTransaction
	}
	return status
}

func (s *clientSession) close() {
	if s.tx != nil {
		_ = s.tx.Rollback()
		s.finishTransaction()
	}
}

func (s *clientSession) initDatabase(client *protocol.Client, database string) error {
	if database == "" || database != s.service.cfg.Upstream.Database {
		return writeError(client, 1, 1049, "42000", "unknown or unconfigured database")
	}
	_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
	return err
}

func (s *clientSession) query(parent context.Context, client *protocol.Client, query string) error {
	statement := engine.ClassifySQL(query)
	if statement.Kind == engine.StatementEmpty {
		return writeError(client, 1, 1065, "42000", "query was empty")
	}
	if statement.Kind == engine.StatementUnsupported {
		return writeError(client, 1, 1235, "42000", statement.Reason)
	}
	_, readTimeout, writeTimeout, _, _ := s.service.cfg.UpstreamDurations()
	limit := readTimeout
	if statement.Kind != engine.StatementRead && writeTimeout < limit {
		limit = writeTimeout
	}
	ctx, cancel := context.WithTimeout(parent, limit)
	defer cancel()

	switch statement.Kind {
	case engine.StatementBegin:
		if s.tx != nil {
			return writeError(client, 1, 1192, "25000", "transaction is already active")
		}
		if err := s.begin(ctx); err != nil {
			return s.writeSQLError(client, err)
		}
		_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
		return err
	case engine.StatementCommit:
		return s.commit(client)
	case engine.StatementRollback:
		return s.rollback(client)
	case engine.StatementAutocommitOff:
		s.autocommit = false
		_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
		return err
	case engine.StatementAutocommitOn:
		if s.tx != nil {
			if err := s.tx.Commit(); err != nil {
				s.finishTransaction()
				return err
			}
			s.finishTransaction()
		}
		s.autocommit = true
		_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
		return err
	case engine.StatementSetNames:
		_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
		return err
	case engine.StatementUseDatabase:
		fields := strings.Fields(statement.SQL)
		if len(fields) != 2 {
			return writeError(client, 1, 1064, "42000", "invalid USE statement")
		}
		name := strings.Trim(fields[1], "`")
		return s.initDatabase(client, name)
	case engine.StatementSavepoint:
		if s.tx == nil {
			return writeError(client, 1, 1305, "42000", "savepoint requires an active transaction")
		}
		return s.exec(ctx, client, statement.SQL, true)
	case engine.StatementDDL:
		if s.tx != nil {
			return writeError(client, 1, 1235, "42000", "implicit-commit DDL is refused inside a pooled transaction")
		}
		return s.exec(ctx, client, statement.SQL, false)
	case engine.StatementWrite:
		if err := s.ensureImplicitTransaction(ctx); err != nil {
			return s.writeSQLError(client, err)
		}
		return s.exec(ctx, client, statement.SQL, s.tx != nil)
	case engine.StatementRead:
		if err := s.ensureImplicitTransaction(ctx); err != nil {
			return s.writeSQLError(client, err)
		}
		return s.rows(ctx, client, statement.SQL)
	default:
		return writeError(client, 1, 1235, "42000", "statement is not supported")
	}
}

func (s *clientSession) begin(ctx context.Context) error {
	release, err := s.service.acquire(ctx, 0)
	if err != nil {
		return err
	}
	txContext, cancel := context.WithCancel(s.context)
	tx, err := s.service.database.BeginTx(txContext, nil)
	if err != nil {
		cancel()
		release()
		return err
	}
	s.tx = tx
	s.txCancel = cancel
	s.release = release
	s.service.pinned.Add(1)
	return nil
}

func (s *clientSession) ensureImplicitTransaction(ctx context.Context) error {
	if !s.autocommit && s.tx == nil {
		return s.begin(ctx)
	}
	return nil
}

func (s *clientSession) finishTransaction() {
	if s.tx != nil {
		s.service.pinned.Add(-1)
	}
	s.tx = nil
	if s.txCancel != nil {
		s.txCancel()
		s.txCancel = nil
	}
	if s.release != nil {
		s.release()
		s.release = nil
	}
}

func (s *clientSession) commit(client *protocol.Client) error {
	if s.tx == nil {
		_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
		return err
	}
	err := s.tx.Commit()
	s.finishTransaction()
	if err != nil {
		return err
	}
	_, err = s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
	return err
}

func (s *clientSession) rollback(client *protocol.Client) error {
	if s.tx != nil {
		err := s.tx.Rollback()
		s.finishTransaction()
		if err != nil && !errors.Is(err, sql.ErrTxDone) {
			return s.writeSQLError(client, err)
		}
	}
	_, err := s.write(client, 1, protocol.OKPayload(0, 0, s.status(), 0))
	return err
}

func (s *clientSession) exec(ctx context.Context, client *protocol.Client, query string, transactional bool) error {
	var result sql.Result
	var warnings uint16
	var err error
	if transactional {
		result, err = s.tx.ExecContext(ctx, query)
		if err == nil {
			warnings, err = readWarningCount(ctx, s.tx)
		}
	} else {
		release, acquireErr := s.service.acquire(ctx, int64(len(query)))
		if acquireErr != nil {
			return s.writeSQLError(client, acquireErr)
		}
		defer release()
		connection, connectionErr := s.service.database.Conn(ctx)
		if connectionErr != nil {
			return s.writeSQLError(client, connectionErr)
		}
		defer connection.Close()
		result, err = connection.ExecContext(ctx, query)
		if err == nil {
			warnings, err = readWarningCount(ctx, connection)
		}
	}
	if err != nil {
		return s.writeSQLError(client, err)
	}
	affected, _ := result.RowsAffected()
	insertID, _ := result.LastInsertId()
	_, err = s.write(client, 1, protocol.OKPayload(uint64(max(0, affected)), uint64(max(0, insertID)), s.status(), warnings))
	return err
}

func (s *clientSession) rows(ctx context.Context, client *protocol.Client, query string) error {
	var rows *sql.Rows
	var warningSource warningCounter
	var err error
	var release func()
	if s.tx != nil {
		rows, err = s.tx.QueryContext(ctx, query)
		warningSource = s.tx
	} else {
		release, err = s.service.acquire(ctx, int64(len(query)))
		if err == nil {
			var connection *sql.Conn
			connection, err = s.service.database.Conn(ctx)
			if err == nil {
				defer connection.Close()
				rows, err = connection.QueryContext(ctx, query)
				warningSource = connection
			}
		}
	}
	if release != nil {
		defer release()
	}
	if err != nil {
		return s.writeSQLError(client, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return s.writeSQLError(client, err)
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return s.writeSQLError(client, err)
	}
	sequence, err := s.write(client, 1, protocol.ColumnCountPayload(uint64(len(columns))))
	if err != nil {
		return err
	}
	for index, name := range columns {
		definition := columnDefinition(name, s.service.cfg.Upstream.Database, types[index])
		sequence, err = s.write(client, sequence, protocol.ColumnDefinitionPayload(definition))
		if err != nil {
			return err
		}
	}
	sequence, err = s.write(client, sequence, protocol.EOFPayload(s.status(), 0))
	if err != nil {
		return err
	}
	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for index := range destinations {
		destinations[index] = &values[index]
	}
	for rows.Next() {
		if err := rows.Scan(destinations...); err != nil {
			return s.writeSQLErrorAt(client, sequence, err)
		}
		encoded := make([][]byte, len(values))
		nulls := make([]bool, len(values))
		for index, value := range values {
			encoded[index], nulls[index] = textValue(value, types[index].DatabaseTypeName())
		}
		sequence, err = s.write(client, sequence, protocol.TextRowPayload(encoded, nulls))
		if err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return s.writeSQLErrorAt(client, sequence, err)
	}
	if err := rows.Close(); err != nil {
		return s.writeSQLErrorAt(client, sequence, err)
	}
	warnings, err := readWarningCount(ctx, warningSource)
	if err != nil {
		return s.writeSQLErrorAt(client, sequence, err)
	}
	_, err = s.write(client, sequence, protocol.EOFPayload(s.status(), warnings))
	return err
}

type warningCounter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readWarningCount(ctx context.Context, source warningCounter) (uint16, error) {
	if source == nil {
		return 0, errors.New("warning source is unavailable")
	}
	var count uint64
	if err := source.QueryRowContext(ctx, "SHOW COUNT(*) WARNINGS").Scan(&count); err != nil {
		return 0, fmt.Errorf("read warning count: %w", err)
	}
	return uint16(min(count, uint64(math.MaxUint16))), nil
}

func (s *clientSession) write(client *protocol.Client, sequence uint8, payload []byte) (uint8, error) {
	s.service.databaseBytes.Add(uint64(len(payload) + protocol.HeaderBytes))
	return client.WriteMessage(sequence, payload)
}

func (s *clientSession) writeSQLError(client *protocol.Client, err error) error {
	return s.writeSQLErrorAt(client, 1, err)
}

func (s *clientSession) writeSQLErrorAt(client *protocol.Client, sequence uint8, err error) error {
	s.service.queryErrors.Add(1)
	code, state, message := mysqlError(err)
	return writeError(client, sequence, code, state, message)
}

func writeError(client *protocol.Client, sequence uint8, code uint16, state, message string) error {
	if len(message) > maxErrorMessageBytes {
		message = message[:maxErrorMessageBytes]
	}
	_, err := client.WriteMessage(sequence, protocol.ErrorPayload(code, state, message))
	return err
}

func mysqlError(err error) (uint16, string, string) {
	var serverError *driver.MySQLError
	if errors.As(err, &serverError) {
		state := string(serverError.SQLState[:])
		if strings.Trim(state, "\x00") == "" {
			state = "HY000"
		}
		return serverError.Number, state, serverError.Message
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return 3024, "HY000", "query deadline exceeded"
	}
	if errors.Is(err, errQueueFull) {
		return 1040, "08004", errQueueFull.Error()
	}
	return 1105, "HY000", err.Error()
}

func columnDefinition(name, database string, column *sql.ColumnType) protocol.ColumnDefinition {
	databaseType := strings.ToUpper(column.DatabaseTypeName())
	definition := protocol.ColumnDefinition{
		Schema:       database,
		Name:         name,
		OriginalName: name,
		Charset:      protocol.DefaultCharsetID,
		Length:       1024,
		Type:         protocol.ColumnTypeVarString,
	}
	if length, ok := column.Length(); ok {
		definition.Length = uint32(min(length, math.MaxUint32))
	}
	if nullable, ok := column.Nullable(); ok && !nullable {
		definition.Flags |= protocol.ColumnFlagNotNull
	}
	if strings.Contains(databaseType, "UNSIGNED") {
		definition.Flags |= protocol.ColumnFlagUnsigned
	}
	definition.Type = mysqlColumnType(databaseType)
	if precision, scale, ok := column.DecimalSize(); ok {
		definition.Decimals = byte(min(scale, 255))
		if definition.Type == protocol.ColumnTypeDecimal || definition.Type == protocol.ColumnTypeNewDecimal {
			wireLength := precision + 1
			if scale > 0 {
				wireLength++
			}
			definition.Length = uint32(min(wireLength, int64(math.MaxUint32)))
		}
	}
	isText := strings.Contains(databaseType, "TEXT")
	if !isText && (definition.Type == protocol.ColumnTypeBlob || definition.Type == protocol.ColumnTypeTinyBlob || definition.Type == protocol.ColumnTypeMediumBlob || definition.Type == protocol.ColumnTypeLongBlob || definition.Type == protocol.ColumnTypeGeometry || definition.Type == protocol.ColumnTypeBit) {
		definition.Charset = 63
		definition.Flags |= protocol.ColumnFlagBinary
	}
	return definition
}

func mysqlColumnType(name string) byte {
	base := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(name, " UNSIGNED"), "UNSIGNED "))
	switch base {
	case "TINYINT", "BOOL", "BOOLEAN":
		return protocol.ColumnTypeTiny
	case "SMALLINT":
		return protocol.ColumnTypeShort
	case "MEDIUMINT":
		return protocol.ColumnTypeInt24
	case "INT", "INTEGER":
		return protocol.ColumnTypeLong
	case "BIGINT":
		return protocol.ColumnTypeLongLong
	case "FLOAT":
		return protocol.ColumnTypeFloat
	case "DOUBLE", "REAL":
		return protocol.ColumnTypeDouble
	case "DECIMAL", "NUMERIC":
		return protocol.ColumnTypeNewDecimal
	case "DATE":
		return protocol.ColumnTypeDate
	case "TIME":
		return protocol.ColumnTypeTime
	case "DATETIME":
		return protocol.ColumnTypeDateTime
	case "TIMESTAMP":
		return protocol.ColumnTypeTimestamp
	case "YEAR":
		return protocol.ColumnTypeYear
	case "BIT":
		return protocol.ColumnTypeBit
	case "JSON":
		return protocol.ColumnTypeJSON
	case "ENUM":
		return protocol.ColumnTypeEnum
	case "SET":
		return protocol.ColumnTypeSet
	case "TINYBLOB":
		return protocol.ColumnTypeTinyBlob
	case "MEDIUMBLOB":
		return protocol.ColumnTypeMediumBlob
	case "LONGBLOB":
		return protocol.ColumnTypeLongBlob
	case "TINYTEXT", "TEXT", "MEDIUMTEXT", "LONGTEXT":
		return protocol.ColumnTypeBlob
	case "BLOB", "BINARY", "VARBINARY":
		return protocol.ColumnTypeBlob
	case "CHAR":
		return protocol.ColumnTypeString
	case "VARCHAR":
		return protocol.ColumnTypeVarChar
	case "GEOMETRY":
		return protocol.ColumnTypeGeometry
	default:
		return protocol.ColumnTypeVarString
	}
}

func textValue(value any, databaseType string) ([]byte, bool) {
	if value == nil {
		return nil, true
	}
	switch typed := value.(type) {
	case []byte:
		return append([]byte(nil), typed...), false
	case string:
		return []byte(typed), false
	case time.Time:
		switch strings.ToUpper(databaseType) {
		case "DATE":
			return []byte(typed.Format("2006-01-02")), false
		case "TIME":
			return []byte(typed.Format("15:04:05.000000")), false
		default:
			return []byte(typed.Format("2006-01-02 15:04:05.000000")), false
		}
	case bool:
		if typed {
			return []byte("1"), false
		}
		return []byte("0"), false
	default:
		return []byte(fmt.Sprint(typed)), false
	}
}
