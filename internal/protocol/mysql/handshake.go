package mysql

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

const (
	ProtocolVersion10 = 10
	DefaultCharsetID  = 45 // utf8mb4_general_ci
	DefaultAuthPlugin = "caching_sha2_password"
)

var (
	ErrMalformedHandshake    = errors.New("malformed mysql handshake response")
	ErrUnsupportedCapability = errors.New("unsupported mysql capability")
	ErrUnsupportedCharset    = errors.New("unsupported mysql client charset")
)

type HandshakeConfig struct {
	ServerVersion      string
	ConnectionID       uint32
	Capabilities       Capability
	CharsetID          byte
	Status             uint16
	AuthPlugin         string
	MaxAttributesBytes int
}

func DefaultHandshakeConfig(connectionID uint32) HandshakeConfig {
	return HandshakeConfig{
		ServerVersion:      "DatabaseAccelerator-0.0.2",
		ConnectionID:       connectionID,
		Capabilities:       DefaultServerCapabilities,
		CharsetID:          DefaultCharsetID,
		Status:             ServerStatusAutocommit,
		AuthPlugin:         DefaultAuthPlugin,
		MaxAttributesBytes: 16 << 10,
	}
}

type Handshake struct {
	config HandshakeConfig
	seed   [20]byte
}

func NewHandshake(config HandshakeConfig) (*Handshake, error) {
	if strings.ContainsRune(config.ServerVersion, 0) || config.ServerVersion == "" {
		return nil, errors.New("mysql server version is invalid")
	}
	if strings.ContainsRune(config.AuthPlugin, 0) || config.AuthPlugin == "" {
		return nil, errors.New("mysql auth plugin is invalid")
	}
	if config.MaxAttributesBytes <= 0 {
		return nil, errors.New("mysql max attributes bytes must be positive")
	}
	if !supportedCharset(config.CharsetID) {
		return nil, ErrUnsupportedCharset
	}
	handshake := &Handshake{config: config}
	if _, err := rand.Read(handshake.seed[:]); err != nil {
		return nil, fmt.Errorf("generate mysql auth seed: %w", err)
	}
	for index, value := range handshake.seed {
		if value == 0 {
			handshake.seed[index] = 1
		}
	}
	return handshake, nil
}

func (h *Handshake) Seed() []byte {
	seed := make([]byte, len(h.seed))
	copy(seed, h.seed[:])
	return seed
}

func (h *Handshake) GreetingPayload() []byte {
	capabilities := uint32(h.config.Capabilities)
	payload := make([]byte, 0, 96)
	payload = append(payload, ProtocolVersion10)
	payload = appendNullTerminated(payload, h.config.ServerVersion)
	var value [4]byte
	binary.LittleEndian.PutUint32(value[:], h.config.ConnectionID)
	payload = append(payload, value[:]...)
	payload = append(payload, h.seed[:8]...)
	payload = append(payload, 0)
	payload = append(payload, byte(capabilities), byte(capabilities>>8))
	payload = append(payload, h.config.CharsetID)
	payload = append(payload, byte(h.config.Status), byte(h.config.Status>>8))
	payload = append(payload, byte(capabilities>>16), byte(capabilities>>24))
	payload = append(payload, byte(len(h.seed)+1))
	payload = append(payload, make([]byte, 10)...)
	payload = append(payload, h.seed[8:]...)
	payload = append(payload, 0)
	payload = appendNullTerminated(payload, h.config.AuthPlugin)
	return payload
}

// SendGreeting writes the initial protocol-v10 greeting with packet sequence 0.
func (h *Handshake) SendGreeting(client *Client) error {
	if client == nil {
		return errors.New("mysql client is required")
	}
	next, err := client.WriteMessage(0, h.GreetingPayload())
	if err != nil {
		return err
	}
	if next != 1 {
		return fmt.Errorf("%w: greeting sequence", ErrSequence)
	}
	return nil
}

// ReadResponse reads the first client handshake response. TLS upgrade is
// recognized but completed by the later TLS/authentication task.
func (h *Handshake) ReadResponse(client *Client) (HandshakeResponse, error) {
	if client == nil {
		return HandshakeResponse{}, errors.New("mysql client is required")
	}
	message, err := client.ReadMessage()
	if err != nil {
		return HandshakeResponse{}, err
	}
	if message.Sequence != 1 {
		return HandshakeResponse{}, fmt.Errorf("%w: handshake got %d want 1", ErrSequence, message.Sequence)
	}
	return h.ParseResponse(message.Payload)
}

type HandshakeResponse struct {
	ClientCapabilities Capability
	Negotiated         Capability
	MaxPacketBytes     uint32
	CharsetID          byte
	Username           string
	AuthResponse       []byte
	Database           string
	AuthPlugin         string
	Attributes         map[string]string
	WantsTLS           bool
}

func (h *Handshake) ParseResponse(payload []byte) (HandshakeResponse, error) {
	if len(payload) < 32 {
		return HandshakeResponse{}, ErrMalformedHandshake
	}
	cursor := newByteCursor(payload)
	capabilitiesRaw, ok := cursor.uint32()
	if !ok {
		return HandshakeResponse{}, ErrMalformedHandshake
	}
	clientCapabilities := Capability(capabilitiesRaw)
	maxPacket, ok := cursor.uint32()
	if !ok {
		return HandshakeResponse{}, ErrMalformedHandshake
	}
	charset, ok := cursor.byte()
	if !ok || !cursor.skip(23) {
		return HandshakeResponse{}, ErrMalformedHandshake
	}
	if err := h.validateCapabilities(clientCapabilities, charset); err != nil {
		return HandshakeResponse{}, err
	}
	response := HandshakeResponse{
		ClientCapabilities: clientCapabilities,
		Negotiated:         clientCapabilities & h.config.Capabilities,
		MaxPacketBytes:     maxPacket,
		CharsetID:          charset,
		Attributes:         make(map[string]string),
	}
	if cursor.remaining() == 0 && clientCapabilities&ClientSSL != 0 {
		if h.config.Capabilities&ClientSSL == 0 {
			return HandshakeResponse{}, fmt.Errorf("%w: tls", ErrUnsupportedCapability)
		}
		response.WantsTLS = true
		return response, nil
	}

	username, ok := cursor.nullTerminated()
	if !ok {
		return HandshakeResponse{}, ErrMalformedHandshake
	}
	response.Username = username

	var authResponse []byte
	var err error
	switch {
	case clientCapabilities&ClientPluginAuthLenencClientData != 0:
		authResponse, err = cursor.lenEncodedBytes()
	case clientCapabilities&ClientSecureConnection != 0:
		length, present := cursor.byte()
		if !present {
			err = ErrMalformedHandshake
			break
		}
		authResponse, ok = cursor.bytes(int(length))
		if !ok {
			err = ErrMalformedHandshake
		}
	default:
		var text string
		text, ok = cursor.nullTerminated()
		if !ok {
			err = ErrMalformedHandshake
		} else {
			authResponse = []byte(text)
		}
	}
	if err != nil {
		return HandshakeResponse{}, err
	}
	response.AuthResponse = append([]byte(nil), authResponse...)

	if clientCapabilities&ClientConnectWithDB != 0 {
		response.Database, ok = cursor.nullTerminated()
		if !ok {
			return HandshakeResponse{}, ErrMalformedHandshake
		}
	}
	if clientCapabilities&ClientPluginAuth != 0 {
		response.AuthPlugin, ok = cursor.nullTerminated()
		if !ok {
			return HandshakeResponse{}, ErrMalformedHandshake
		}
	}
	if clientCapabilities&ClientConnectAttrs != 0 {
		attributeBytes, attributeErr := cursor.lenEncodedBytes()
		if attributeErr != nil || len(attributeBytes) > h.config.MaxAttributesBytes {
			return HandshakeResponse{}, ErrMalformedHandshake
		}
		attributeCursor := newByteCursor(attributeBytes)
		for attributeCursor.remaining() > 0 {
			key, keyErr := attributeCursor.lenEncodedBytes()
			value, valueErr := attributeCursor.lenEncodedBytes()
			if keyErr != nil || valueErr != nil || len(key) == 0 {
				return HandshakeResponse{}, ErrMalformedHandshake
			}
			response.Attributes[string(key)] = string(value)
		}
	}
	if cursor.remaining() != 0 {
		return HandshakeResponse{}, ErrMalformedHandshake
	}
	if response.AuthPlugin == "" {
		response.AuthPlugin = h.config.AuthPlugin
	}
	return response, nil
}

func (h *Handshake) validateCapabilities(client Capability, charset byte) error {
	required := ClientProtocol41 | ClientSecureConnection | ClientPluginAuth
	if client&required != required {
		return fmt.Errorf("%w: required protocol flags", ErrUnsupportedCapability)
	}
	if client&(ClientCompress|ClientZstdCompressionAlgorithm) != 0 {
		return fmt.Errorf("%w: compression", ErrUnsupportedCapability)
	}
	unsupported := client &^ h.config.Capabilities
	permittedClientOnly := ClientSSL | ClientFoundRows | ClientLocalFiles | ClientMultiStatements | ClientSessionTrack | ClientRememberOptions
	if unsupported&^permittedClientOnly != 0 {
		return fmt.Errorf("%w: unknown requested flag 0x%08x", ErrUnsupportedCapability, uint32(unsupported&^permittedClientOnly))
	}
	if !supportedCharset(charset) {
		return ErrUnsupportedCharset
	}
	return nil
}

func supportedCharset(charset byte) bool {
	switch charset {
	case 8, 33, 45, 46, 224, 255:
		return true
	default:
		return false
	}
}

// SafeConnectionAttributes returns only bounded operational attributes.
func SafeConnectionAttributes(attributes map[string]string) map[string]string {
	allowed := map[string]struct{}{
		"_client_name":    {},
		"_client_version": {},
		"program_name":    {},
		"platform":        {},
	}
	result := make(map[string]string)
	for key, value := range attributes {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if len(value) > 128 {
			value = value[:128]
		}
		result[key] = value
	}
	return result
}

func appendNullTerminated(target []byte, value string) []byte {
	target = append(target, value...)
	return append(target, 0)
}

type byteCursor struct {
	data   []byte
	offset int
}

func newByteCursor(data []byte) *byteCursor { return &byteCursor{data: data} }

func (c *byteCursor) remaining() int { return len(c.data) - c.offset }

func (c *byteCursor) skip(length int) bool {
	if length < 0 || length > c.remaining() {
		return false
	}
	c.offset += length
	return true
}

func (c *byteCursor) byte() (byte, bool) {
	if c.remaining() < 1 {
		return 0, false
	}
	value := c.data[c.offset]
	c.offset++
	return value, true
}

func (c *byteCursor) uint32() (uint32, bool) {
	if c.remaining() < 4 {
		return 0, false
	}
	value := binary.LittleEndian.Uint32(c.data[c.offset : c.offset+4])
	c.offset += 4
	return value, true
}

func (c *byteCursor) bytes(length int) ([]byte, bool) {
	if length < 0 || length > c.remaining() {
		return nil, false
	}
	value := c.data[c.offset : c.offset+length]
	c.offset += length
	return value, true
}

func (c *byteCursor) nullTerminated() (string, bool) {
	for index := c.offset; index < len(c.data); index++ {
		if c.data[index] == 0 {
			value := string(c.data[c.offset:index])
			c.offset = index + 1
			return value, true
		}
	}
	return "", false
}

func (c *byteCursor) lenEncodedBytes() ([]byte, error) {
	length, err := c.lenEncodedInt()
	if err != nil {
		return nil, err
	}
	if length > uint64(c.remaining()) || length > uint64(^uint(0)>>1) {
		return nil, ErrMalformedHandshake
	}
	value, ok := c.bytes(int(length))
	if !ok {
		return nil, ErrMalformedHandshake
	}
	return value, nil
}

func (c *byteCursor) lenEncodedInt() (uint64, error) {
	first, ok := c.byte()
	if !ok {
		return 0, ErrMalformedHandshake
	}
	switch first {
	case 0xfc:
		value, ok := c.bytes(2)
		if !ok {
			return 0, ErrMalformedHandshake
		}
		return uint64(binary.LittleEndian.Uint16(value)), nil
	case 0xfd:
		value, ok := c.bytes(3)
		if !ok {
			return 0, ErrMalformedHandshake
		}
		return uint64(value[0]) | uint64(value[1])<<8 | uint64(value[2])<<16, nil
	case 0xfe:
		value, ok := c.bytes(8)
		if !ok {
			return 0, ErrMalformedHandshake
		}
		return binary.LittleEndian.Uint64(value), nil
	case 0xfb, 0xff:
		return 0, ErrMalformedHandshake
	default:
		return uint64(first), nil
	}
}
