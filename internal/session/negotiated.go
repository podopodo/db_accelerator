package session

import "github.com/podopodo/db_accelerator/internal/protocol/mysql"

// Negotiated is the non-secret client state established during handshake.
// Authentication response bytes are deliberately not retained here.
type Negotiated struct {
	Capabilities mysql.Capability
	MaxPacket    uint32
	CharsetID    byte
	Username     string
	Database     string
	AuthPlugin   string
	Attributes   map[string]string
}

func FromHandshake(response mysql.HandshakeResponse) Negotiated {
	return Negotiated{
		Capabilities: response.Negotiated,
		MaxPacket:    response.MaxPacketBytes,
		CharsetID:    response.CharsetID,
		Username:     response.Username,
		Database:     response.Database,
		AuthPlugin:   response.AuthPlugin,
		Attributes:   mysql.SafeConnectionAttributes(response.Attributes),
	}
}
