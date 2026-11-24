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

// Package fake has a test double for target.API that replays
// recorded controller responses and record what was sent.
//
// Responses recorded from real controllers live in testdata/<version>/, one
// file per path: the path without its leading slash, with "/" as "_", and
// .json, .txt or .html after it ("/api/system/info" is
// api_system_info.json).
package fake

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// Request is one request the code under test sent.
type Request struct {
	Method string
	Path   string
	Body   string
}

// API is a fake target.API. Replies are keyed by method and exact path.
// A request with no reply gets 404, and the test fails at cleanup unless
// AllowUnexpected was called.
type API struct {
	t testing.TB

	mu         sync.Mutex
	replies    map[string][]*target.Response
	requests   []Request
	unexpected []Request
	allow      bool
	addr       string
	passwords  []string
}

var _ target.API = (*API)(nil)

// NewAPI returns an API with no replies.
func NewAPI(t testing.TB) *API {
	f := &API{t: t, replies: map[string][]*target.Response{}}
	t.Cleanup(f.checkUnexpected)
	return f
}

// Reply adds a reply to method on path. Several replies to one request are
// given in order, and the last repeats (for polling: 503, then 200).
func (f *API) Reply(method, path string, status int, body string) *API {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := key(method, path)
	f.replies[k] = append(f.replies[k], &target.Response{Status: status, Body: []byte(body)})
	return f
}

// ReplyFile is Reply with the body read from testdata (file is relative to
// it, as "fpp10.4/api_system_info.json").
func (f *API) ReplyFile(method, path string, status int, file string) *API {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(Testdata(), file))
	if err != nil {
		f.t.Fatalf("fake: %v", err)
	}
	return f.Reply(method, path, status, string(b))
}

// Recorded adds 200 replies to GET on each path, from the responses recorded
// from a controller running version ("fpp10.4").
func (f *API) Recorded(version string, paths ...string) *API {
	f.t.Helper()
	for _, p := range paths {
		f.ReplyFile("GET", p, 200, filepath.Join(version, fixture(version, p)))
	}
	return f
}

// fixture finds the recorded file for path.
func fixture(version, path string) string {
	base := strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "_")
	for _, ext := range []string{".json", ".txt", ".html"} {
		if _, err := os.Stat(filepath.Join(Testdata(), version, base+ext)); err == nil {
			return base + ext
		}
	}
	return base + ".json" // ReplyFile reports it missing
}

// AllowUnexpected lets requests with no reply through as 404 without failing
// the test.
func (f *API) AllowUnexpected() *API {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allow = true
	return f
}

// Do records the request and returns its reply.
func (f *API) Do(ctx context.Context, method, path string, body io.Reader, contentType string) (*target.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req := Request{Method: method, Path: path}
	if body != nil {
		b, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		req.Body = string(b)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	queue := f.replies[key(method, path)]
	if len(queue) == 0 {
		f.unexpected = append(f.unexpected, req)
		return &target.Response{Status: 404, Body: []byte(`{"status":"ERROR","message":"fake: no reply"}`)}, nil
	}
	if len(queue) > 1 {
		f.replies[key(method, path)] = queue[1:]
	}
	r := queue[0]
	return &target.Response{Status: r.Status, Body: r.Body}, nil
}

// SetAddress records the address; see Address.
func (f *API) SetAddress(addr string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addr = addr
}

// AddPassword records the password; see Passwords.
func (f *API) AddPassword(password string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.passwords = append(f.passwords, password)
}

// Requests returns every request sent, in order.
func (f *API) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// Writes returns the requests other than GET, in order.
func (f *API) Writes() []Request {
	var out []Request
	for _, r := range f.Requests() {
		if r.Method != "GET" {
			out = append(out, r)
		}
	}
	return out
}

// Unexpected returns the requests that had no reply.
func (f *API) Unexpected() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.unexpected...)
}

// Address returns the last address set, or "".
func (f *API) Address() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addr
}

// Passwords returns the passwords added, in order.
func (f *API) Passwords() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.passwords...)
}

func (f *API) checkUnexpected() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.allow {
		return
	}
	for _, r := range f.unexpected {
		f.t.Errorf("fake API: unexpected %s %s", r.Method, r.Path)
	}
}

func key(method, path string) string { return method + " " + path }

// Testdata returns the absolute path of this package's testdata directory,
// so tests in other packages can find the recorded responses.
func Testdata() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}
