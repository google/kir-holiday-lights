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

package fpp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// controller is a stand-in FPP that wants password ("" for none) and records
// the password each request carried ("-" for none).
type controller struct {
	password string
	seen     []string
}

func (c *controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_, got, ok := r.BasicAuth()
	if ok {
		c.seen = append(c.seen, got)
	} else {
		c.seen = append(c.seen, "-")
	}
	if c.password != "" && (!ok || got != c.password) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Falcon Player"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
	w.Write([]byte(r.Method + " " + r.URL.Path + " " + string(body)))
}

// start serves c and returns a client for it with passwords.
func start(t *testing.T, c *controller, passwords ...string) *Client {
	t.Helper()
	srv := httptest.NewServer(c)
	t.Cleanup(srv.Close)
	return New(strings.TrimPrefix(srv.URL, "http://"), passwords)
}

func get(t *testing.T, cl *Client, wantStatus int) {
	t.Helper()
	resp, err := cl.Do(context.Background(), "GET", "/api/system/info", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != wantStatus {
		t.Fatalf("status %d, want %d", resp.Status, wantStatus)
	}
}

func assertSeen(t *testing.T, c *controller, want ...string) {
	t.Helper()
	if strings.Join(c.seen, ",") != strings.Join(want, ",") {
		t.Errorf("credentials sent %q, want %q", c.seen, want)
	}
}

func TestNoPasswordNeeded(t *testing.T) {
	c := &controller{}
	cl := start(t, c, "config")
	get(t, cl, 200)
	get(t, cl, 200)
	assertSeen(t, c, "-", "-")
}

func TestConfigPasswordWorks(t *testing.T) {
	c := &controller{password: "config"}
	cl := start(t, c, "config", "old")
	get(t, cl, 200)
	get(t, cl, 200)
	assertSeen(t, c, "-", "config", "config")
}

func TestSecondPasswordWorks(t *testing.T) {
	c := &controller{password: "old"}
	cl := start(t, c, "config", "old")
	get(t, cl, 200)
	get(t, cl, 200)
	assertSeen(t, c, "-", "config", "old", "old")
}

func TestAllRefused(t *testing.T) {
	c := &controller{password: "other"}
	cl := start(t, c, "config", "old")
	get(t, cl, 401)
	assertSeen(t, c, "-", "config", "old")
}

func TestAddPassword(t *testing.T) {
	c := &controller{password: "config"}
	cl := start(t, c, "config")
	get(t, cl, 200)
	// The tool changes the UI password, then tells the client.
	c.password = "new"
	cl.AddPassword("new")
	get(t, cl, 200)
	get(t, cl, 200)
	assertSeen(t, c, "-", "config", "config", "new", "new")
}

func TestBodyResentOnRetry(t *testing.T) {
	c := &controller{password: "config"}
	cl := start(t, c, "config")
	resp, err := cl.Do(context.Background(), "PUT", "/api/settings/HostName", strings.NewReader("fpp-bench"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(resp.Body); got != "PUT /api/settings/HostName fpp-bench" {
		t.Errorf("body %q", got)
	}
}

func TestHostPart(t *testing.T) {
	for addr, want := range map[string]string{
		"192.168.1.20":   "192.168.1.20",
		"fe80::1%eth0":   "[fe80::1%25eth0]",
		"fe80::1":        "[fe80::1]",
		"127.0.0.1:8080": "127.0.0.1:8080", // tests' servers
	} {
		if got := hostPart(addr); got != want {
			t.Errorf("hostPart(%q) = %q, want %q", addr, got, want)
		}
	}
	// The URL must parse back to the zone.
	req, err := http.NewRequest("GET", "http://"+hostPart("fe80::1%eth0")+"/api/system/info", nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Hostname() != "fe80::1%eth0" {
		t.Errorf("hostname %q", req.URL.Hostname())
	}
}

func TestNoPasswordInErrors(t *testing.T) {
	// Once the client is using the password, close the server so the next
	// request fails in transport.
	c := &controller{password: "secret-pw"}
	srv := httptest.NewServer(c)
	cl := New(strings.TrimPrefix(srv.URL, "http://"), []string{"secret-pw"})
	get(t, cl, 200)
	srv.Close()
	_, err := cl.Do(context.Background(), "GET", "/api/system/info", nil, "")
	if err == nil {
		t.Fatal("no error from a closed server")
	}
	if strings.Contains(err.Error(), "secret-pw") {
		t.Errorf("password in error: %v", err)
	}
}

func TestContextDeadlineWins(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	cl := New(strings.TrimPrefix(srv.URL, "http://"), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := cl.Do(ctx, "GET", "/", nil, ""); err == nil {
		t.Fatal("no timeout")
	}
}
