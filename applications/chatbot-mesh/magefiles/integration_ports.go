// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

type portForwardPair struct {
	local  int
	remote int
}

func reserveLoopbackPort() (int, error) {
	address, err := freeLoopbackAddr()
	if err != nil {
		return 0, err
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return 0, err
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("invalid reserved loopback port %q", port)
	}
	return number, nil
}

func reserveLoopbackPorts(count int) ([]int, error) {
	ports := make([]int, 0, count)
	seen := make(map[int]bool, count)
	for len(ports) < count {
		port, err := reserveLoopbackPort()
		if err != nil {
			return nil, err
		}
		if seen[port] {
			continue
		}
		seen[port] = true
		ports = append(ports, port)
	}
	return ports, nil
}

func kubectlPortForwardPairs(
	commands kindrig.Commands,
	target string,
	pairs ...portForwardPair,
) (func(), error) {
	args := portForwardArgs(target, pairs)
	command := commands.Command("kubectl", args...)
	command.Stdout, command.Stderr = os.Stderr, os.Stderr
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("kubectl port-forward %s: %w", target, err)
	}
	return func() {
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}, nil
}

func portForwardArgs(target string, pairs []portForwardPair) []string {
	args := []string{"port-forward", target}
	for _, pair := range pairs {
		args = append(args, fmt.Sprintf("%d:%d", pair.local, pair.remote))
	}
	return args
}

func loopbackURL(port int, path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
}
