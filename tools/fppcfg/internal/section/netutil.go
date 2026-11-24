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

package section

import (
	"context"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// netAddr parses addr, an IP address with or without a zone, into the
// address alone: the zone dropped and an IPv4-mapped address unmapped. ok is
// false for a hostname.
func netAddr(addr string) (ip netip.Addr, ok bool) {
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.WithZone("").Unmap(), true
}

// netHost is addr without its zone, or addr itself when it is a hostname.
func netHost(addr string) string {
	if ip, ok := netAddr(addr); ok {
		return ip.String()
	}
	return addr
}

// netLinkLocal reports whether addr (with or without a zone) is an IPv6
// link-local address.
func netLinkLocal(addr string) bool {
	ip, ok := netAddr(addr)
	return ok && ip.Is6() && ip.IsLinkLocalUnicast()
}

// netGetStatus GETs url with t's plain HTTP client and returns the status,
// or 0 when nothing answered within a few seconds.
func netGetStatus(ctx context.Context, t *target.Target, url string) int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0
	}
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode
}

// netLines splits a file into its lines, without trailing whitespace and
// without blank lines.
func netLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(l, " \t\r")
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
