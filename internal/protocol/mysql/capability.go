package mysql

// Capability is the 32-bit MySQL client/server capability bitmap.
type Capability uint32

const (
	ClientLongPassword               Capability = 1 << 0
	ClientFoundRows                  Capability = 1 << 1
	ClientLongFlag                   Capability = 1 << 2
	ClientConnectWithDB              Capability = 1 << 3
	ClientCompress                   Capability = 1 << 5
	ClientLocalFiles                 Capability = 1 << 7
	ClientProtocol41                 Capability = 1 << 9
	ClientSSL                        Capability = 1 << 11
	ClientTransactions               Capability = 1 << 13
	ClientSecureConnection           Capability = 1 << 15
	ClientMultiStatements            Capability = 1 << 16
	ClientMultiResults               Capability = 1 << 17
	ClientPSMultiResults             Capability = 1 << 18
	ClientPluginAuth                 Capability = 1 << 19
	ClientConnectAttrs               Capability = 1 << 20
	ClientPluginAuthLenencClientData Capability = 1 << 21
	ClientSessionTrack               Capability = 1 << 23
	ClientDeprecateEOF               Capability = 1 << 24
	ClientZstdCompressionAlgorithm   Capability = 1 << 26
)

const DefaultServerCapabilities = ClientLongPassword |
	ClientLongFlag |
	ClientConnectWithDB |
	ClientProtocol41 |
	ClientTransactions |
	ClientSecureConnection |
	ClientMultiResults |
	ClientPSMultiResults |
	ClientPluginAuth |
	ClientConnectAttrs |
	ClientPluginAuthLenencClientData |
	ClientSessionTrack |
	ClientDeprecateEOF

const (
	ServerStatusAutocommit uint16 = 0x0002
)
