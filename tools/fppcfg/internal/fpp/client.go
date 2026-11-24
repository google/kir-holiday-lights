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

// Package fpp is a client for FPP's HTTP API.
package fpp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// defaultTimeout bounds a request whose context has no deadline. Callers that
// need longer (a network apply takes about 40 s) pass a context with their
// own deadline.
const defaultTimeout = 20 * time.Second

// user is the Basic auth user. FPP accepts admin or fpp; only the password
// is checked.
const user = "admin"

// Client implements target.API.
//
// Login policy: requests go out without credentials until FPP answers 401.
// Then the client tries each password in order, then none, and keeps the
// first that FPP accepts for later requests. If all are refused it returns
// the 401 for the caller to report.
//
// Passwords never appear in errors or logs: they travel only in the
// Authorization header, which net/http does not print.
type Client struct {
	http      *http.Client
	base      string   // "http://host"
	passwords []string // to try on a 401, in order
	using     *string  // the credential in use; nil for none
}

var _ target.API = (*Client)(nil)

// New returns a client for the controller at addr: an IPv4 address, or an
// IPv6 address with its zone ("fe80::1%eth0"). passwords are tried in order
// on a 401; empty ones are skipped.
func New(addr string, passwords []string) *Client {
	c := &Client{http: &http.Client{}}
	c.SetAddress(addr)
	for _, p := range slices.Backward(passwords) {
		c.AddPassword(p)
	}
	return c
}

// SetAddress points the client at a new address. The credential in use is
// kept: the controller is the same.
func (c *Client) SetAddress(addr string) {
	c.base = "http://" + hostPart(addr)
}

// hostPart puts an IPv6 address in brackets and escapes its zone, so
// "fe80::1%eth0" becomes "[fe80::1%25eth0]". Anything else is used as is,
// which lets tests give a host:port.
func hostPart(addr string) string {
	if ip, err := netip.ParseAddr(addr); err != nil || !ip.Is6() {
		return addr
	}
	return "[" + strings.ReplaceAll(addr, "%", "%25") + "]"
}

// AddPassword makes password the first one tried on the next 401. It is used
// when the tool changes the UI password mid-run.
func (c *Client) AddPassword(password string) {
	if password == "" {
		return
	}
	c.passwords = slices.DeleteFunc(c.passwords, func(p string) bool { return p == password })
	c.passwords = slices.Insert(c.passwords, 0, password)
}

// Do sends one request, logging in again on a 401 as the type describes.
func (c *Client) Do(ctx context.Context, method, path string, body io.Reader, contentType string) (*target.Response, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}
	// The body is read once so that it can be sent again on a retry.
	var data []byte
	if body != nil {
		var err error
		if data, err = io.ReadAll(body); err != nil {
			return nil, err
		}
	}

	base, using := c.base, c.using
	resp, err := c.send(ctx, base, method, path, data, contentType, using)
	if err != nil || resp.Status != http.StatusUnauthorized {
		return resp, err
	}
	for _, cred := range c.candidates(using) {
		retry, err := c.send(ctx, base, method, path, data, contentType, cred)
		if err != nil {
			return nil, err
		}
		if retry.Status != http.StatusUnauthorized {
			c.using = cred
			return retry, nil
		}
		resp = retry
	}
	return resp, nil
}

// candidates lists the credentials to try after tried was refused: each
// password in order, then none.
func (c *Client) candidates(tried *string) []*string {
	var out []*string
	for _, p := range c.passwords {
		if tried == nil || *tried != p {
			out = append(out, &p)
		}
	}
	if tried != nil {
		out = append(out, nil)
	}
	return out
}

// send makes one attempt with one credential (nil for none).
func (c *Client) send(ctx context.Context, base, method, path string, data []byte, contentType string, cred *string) (*target.Response, error) {
	var body io.Reader
	if data != nil {
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cred != nil {
		req.SetBasicAuth(user, *cred)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &target.Response{Status: resp.StatusCode, Body: b}, nil
}
