// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package servercmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

const (
	LaunchReceiptStrategy       = "compensating_action"
	maxServerLaunchReceiptBytes = 4096
	OwnershipBytes              = 32
)

// LaunchReceipt is the JSON body a server-launch command stores for undo.
type LaunchReceipt struct {
	Strategy    string `json:"strategy"`
	Declaration string `json:"declaration"`
	Server      string `json:"server"`
	Address     string `json:"address"`
	Ownership   string `json:"ownership"`
}

type awaitReceipt struct {
	Server string `json:"server"`
	Event  Event  `json:"event"`
}

// DecodeLaunchReceipt parses a server-launch undo receipt.
func DecodeLaunchReceipt(value string) (LaunchReceipt, error) {
	if value == "" {
		return LaunchReceipt{}, fmt.Errorf("receipt is required")
	}
	if len(value) > maxServerLaunchReceiptBytes {
		return LaunchReceipt{}, fmt.Errorf("receipt exceeds %d bytes", maxServerLaunchReceiptBytes)
	}
	var receipt LaunchReceipt
	decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return LaunchReceipt{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return LaunchReceipt{}, fmt.Errorf("multiple JSON values")
		}
		return LaunchReceipt{}, err
	}
	return canonicalLaunchReceipt(receipt)
}

func canonicalLaunchReceipt(receipt LaunchReceipt) (LaunchReceipt, error) {
	for name, field := range map[string]string{
		"strategy": receipt.Strategy, "declaration": receipt.Declaration,
		"server": receipt.Server, "address": receipt.Address, "ownership": receipt.Ownership,
	} {
		if field == "" || strings.TrimSpace(field) != field {
			return LaunchReceipt{}, fmt.Errorf("%s is required and must be canonical", name)
		}
	}
	return receipt, nil
}

func validateLaunchAddress(configured, actual string) error {
	actualHost, actualPortText, err := net.SplitHostPort(actual)
	if err != nil {
		return fmt.Errorf("invalid bound address %q: %w", actual, err)
	}
	actualPort, err := strconv.Atoi(actualPortText)
	if err != nil || actualPort < 1 || actualPort > 65535 {
		return fmt.Errorf("invalid bound port in %q", actual)
	}
	return matchConfiguredAddress(configured, actual, actualHost, actualPort)
}

func matchConfiguredAddress(configured, actual, actualHost string, actualPort int) error {
	configuredHost, configuredPortText, err := net.SplitHostPort(configured)
	if err != nil {
		return fmt.Errorf("invalid configured address %q: %w", configured, err)
	}
	configuredPort, err := strconv.Atoi(configuredPortText)
	if err != nil || configuredPort < 0 || configuredPort > 65535 {
		return fmt.Errorf("invalid configured port in %q", configured)
	}
	if configuredPort != 0 && configuredPort != actualPort {
		return fmt.Errorf("bound address %q does not match configured address %q", actual, configured)
	}
	configuredIP, actualIP := net.ParseIP(configuredHost), net.ParseIP(actualHost)
	if configuredIP != nil && !configuredIP.IsUnspecified() &&
		(actualIP == nil || !configuredIP.Equal(actualIP)) {
		return fmt.Errorf("bound address %q does not match configured address %q", actual, configured)
	}
	return nil
}
