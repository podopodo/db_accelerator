package session

import (
	"testing"

	"github.com/podopodo/db_accelerator/internal/protocol/mysql"
)

func TestFromHandshakeStoresOnlySafeNegotiatedState(t *testing.T) {
	state := FromHandshake(mysql.HandshakeResponse{
		Negotiated:     mysql.ClientProtocol41 | mysql.ClientSecureConnection,
		MaxPacketBytes: 1 << 20,
		CharsetID:      45,
		Username:       "app",
		Database:       "app_db",
		AuthPlugin:     mysql.DefaultAuthPlugin,
		AuthResponse:   []byte("secret-response"),
		Attributes: map[string]string{
			"_client_name": "driver",
			"password":     "do-not-retain",
		},
	})
	if state.Username != "app" || state.Database != "app_db" || state.Attributes["_client_name"] != "driver" {
		t.Fatalf("state = %+v", state)
	}
	if _, ok := state.Attributes["password"]; ok {
		t.Fatal("unsafe attribute retained")
	}
}
