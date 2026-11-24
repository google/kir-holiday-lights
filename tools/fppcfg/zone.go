// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

// zoner adds the operator's interface to a link-local IPv6 address that has
// none. The zone names an interface on this machine, so a config cannot
// carry it.
type zoner struct {
	// interfaces lists this machine's interfaces that are up and have an
	// IPv6 link-local address.
	interfaces func() ([]string, error)
	// answers reports whether addr (with zone) accepts a TCP connection on
	// port 80 before ctx ends.
	answers func(ctx context.Context, addr string) bool
	timeout time.Duration
}

func newZoner() *zoner {
	return &zoner{interfaces: linkLocalInterfaces, answers: dialHTTP, timeout: 3 * time.Second}
}

// withZone returns addr unchanged unless it is link-local IPv6 without a
// zone. Then it uses the only candidate interface, or among several the one
// through which the controller answers.
func (z *zoner) withZone(ctx context.Context, addr string) (string, error) {
	ip, err := netip.ParseAddr(addr)
	if err != nil || !ip.Is6() || !ip.IsLinkLocalUnicast() || ip.Zone() != "" {
		return addr, nil
	}
	names, err := z.interfaces()
	if err != nil {
		return "", fmt.Errorf("listing interfaces for %s: %w", addr, err)
	}
	switch len(names) {
	case 0:
		return "", fmt.Errorf("%s is link-local, and no interface here has IPv6 link-local up", addr)
	case 1:
		return addr + "%" + names[0], nil
	}

	ctx, cancel := context.WithTimeout(ctx, z.timeout)
	defer cancel()
	ok := make([]bool, len(names))
	done := make(chan struct{})
	for i, n := range names {
		go func() {
			ok[i] = z.answers(ctx, addr+"%"+n)
			done <- struct{}{}
		}()
	}
	for range names {
		<-done
	}
	var found []string
	for i, n := range names {
		if ok[i] {
			found = append(found, n)
		}
	}
	switch len(found) {
	case 1:
		return addr + "%" + found[0], nil
	case 0:
		return "", fmt.Errorf("%s does not answer through any of %s; use --at %s%%IFACE",
			addr, strings.Join(names, ", "), addr)
	default:
		return "", fmt.Errorf("%s answers through %s; use --at %s%%IFACE to choose",
			addr, strings.Join(found, " and "), addr)
	}
}

func linkLocalInterfaces() ([]string, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() == nil && n.IP.IsLinkLocalUnicast() {
				names = append(names, i.Name)
				break
			}
		}
	}
	return names, nil
}

func dialHTTP(ctx context.Context, addr string) bool {
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(addr, "80"))
	if err != nil {
		return false
	}
	c.Close()
	return true
}
