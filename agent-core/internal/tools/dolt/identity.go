// Copyright (c) 2026 Nokia. All rights reserved.

package dolt

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type DatabaseIdentity struct {
	Server   string
	Database string
}

func IdentityFromDSN(dsn, database string) (DatabaseIdentity, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return DatabaseIdentity{}, fmt.Errorf("parse Dolt connection: %w", err)
	}
	if database == "" {
		database = cfg.DBName
	}
	if !literalIdentifier.MatchString(database) {
		return DatabaseIdentity{}, fmt.Errorf("database must be a literal SQL identifier")
	}
	server, err := normalizeServer(cfg.Net, cfg.Addr)
	if err != nil {
		return DatabaseIdentity{}, err
	}
	return DatabaseIdentity{Server: server, Database: strings.ToLower(database)}, nil
}
func normalizeServer(network, address string) (string, error) {
	network = strings.ToLower(strings.TrimSpace(network))
	if network == "unix" {
		if strings.TrimSpace(address) == "" {
			return "", fmt.Errorf("normalize Dolt server address: empty unix socket")
		}
		return "unix://" + filepath.Clean(address), nil
	}
	if network != "" && network != "tcp" && network != "tcp4" && network != "tcp6" {
		return "", fmt.Errorf("normalize Dolt server address: unsupported network %q", network)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port in address") {
			return "", fmt.Errorf("normalize Dolt server address: %w", err)
		}
		host, port = address, "3306"
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "3306"
	}
	return "tcp://" + net.JoinHostPort(host, port), nil
}
func (i DatabaseIdentity) SameServer(o DatabaseIdentity) bool {
	return i.Server != "" && i.Server == o.Server
}
func (i DatabaseIdentity) SameDatabase(o DatabaseIdentity) bool {
	return i.SameServer(o) && i.Database != "" && i.Database == o.Database
}
