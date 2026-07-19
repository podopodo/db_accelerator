package gateway

import "github.com/podopodo/db_accelerator/internal/config"

// permissionIdentity is deliberately comparable so any future multi-pool map
// must separate database handles across client identity, upstream grants,
// selected schema, and transport policy.
type permissionIdentity struct {
	ClientUser      string
	UpstreamHost    string
	UpstreamPort    int
	UpstreamUser    string
	Database        string
	UpstreamTLSMode string
	ClientTLSMode   string
}

func newPermissionIdentity(cfg config.Config) permissionIdentity {
	return permissionIdentity{
		ClientUser:      cfg.Server.MySQLClientUser,
		UpstreamHost:    cfg.Upstream.Host,
		UpstreamPort:    cfg.Upstream.Port,
		UpstreamUser:    cfg.Upstream.User,
		Database:        cfg.Upstream.Database,
		UpstreamTLSMode: cfg.Upstream.TLSMode,
		ClientTLSMode:   cfg.Server.MySQLTLSMode,
	}
}
