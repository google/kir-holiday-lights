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

// Package target is the controller as the sections see it: an API client,
// and how it is reached.
package target

import (
	"context"
	"io"
	"net/http"
)

// API is FPP's HTTP API. Paths start with "/api/" (or "/" for UI pages) and
// are relative to the controller's current address. The client handles
// Basic auth itself: on a 401 it tries its other credentials before
// returning one.
type API interface {
	// Do sends one request. body may be nil. A response with any status is
	// returned without error; err is for transport failures, including a
	// timeout.
	Do(ctx context.Context, method, path string, body io.Reader, contentType string) (*Response, error)
	// SetAddress points the client at a new address (after a network apply).
	SetAddress(addr string)
	// AddPassword adds a credential to try on a 401, and makes it the first
	// one tried. Used when the tool changes the UI password mid-run.
	AddPassword(password string)
}

// Response is a fully read HTTP response.
type Response struct {
	Status int
	Body   []byte
}

// OK reports a 2xx status.
func (r *Response) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Target is one controller for one run.
type Target struct {
	API API
	// Address is how the controller is reached now: an IPv4 address, or an
	// IPv6 address with its zone ("fe80::1%eth0").
	Address string
	// HTTP is a plain client for requests outside FPP's API client, such as
	// checking that the controller answers at a new address.
	HTTP *http.Client
}

// Setting is one setting as FPP describes it, from GET /api/settings/<Name>.
type Setting struct {
	// Defined is false when FPP has no definition for the name (the reply
	// has no "name"), meaning this FPP does not have the setting.
	Defined bool
	// Value is the setting's value as a string, FPP's default when unset. A
	// checkbox that is unset and has no default reads as unchecked ("0").
	Value   string
	Restart int // 0 none, 1 full restart, 2 quick restart
	Reboot  int // 0 or 1
	// Options are the values FPP offers, or nil when it lists none.
	Options []string
}
