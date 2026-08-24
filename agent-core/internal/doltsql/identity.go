// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltsql

import (
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// IdentifierPattern is the unquoted SQL identifier grammar shared by Dolt
// database names and tool operation names.
var IdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// DatabaseIdentity is the credential-free server-and-database identity used to
// keep a Dolt word from sharing a database with the checkpoint backend.
type DatabaseIdentity struct {
	Server   string
	Database string
}

// IdentityFromDSN parses a MySQL-wire DSN and optional database override into a
// normalized identity. Credentials and query parameters are discarded.
func IdentityFromDSN(dsn, database string) (DatabaseIdentity, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return DatabaseIdentity{}, fmt.Errorf("parse Dolt connection: %w", err)
	}
	if database == "" {
		database = cfg.DBName
	}
	if !IdentifierPattern.MatchString(database) {
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

// SameServer reports whether two identities name the same normalized server.
func (i DatabaseIdentity) SameServer(o DatabaseIdentity) bool {
	return i.Server != "" && i.Server == o.Server
}

// SameDatabase reports whether two identities name the same server and database.
func (i DatabaseIdentity) SameDatabase(o DatabaseIdentity) bool {
	return i.SameServer(o) && i.Database != "" && i.Database == o.Database
}
