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

package engine

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fake"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/section"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// stub is a section with fixed state that logs what it was asked to do.
type stub struct {
	name       string
	want, have state.State
	after      section.After
	log        *[]string
	err        error
}

func (s *stub) Name() string      { return s.name }
func (s *stub) Want() state.State { return s.want }
func (s *stub) Observe(context.Context, *target.Target) (state.State, error) {
	*s.log = append(*s.log, "observe "+s.name)
	return s.have, nil
}
func (s *stub) Apply(_ context.Context, _ *target.Target, ch []state.Change) (section.After, error) {
	var keys []string
	for _, c := range ch {
		keys = append(keys, c.Key)
	}
	*s.log = append(*s.log, "apply "+s.name+" "+strings.Join(keys, ","))
	return s.after, s.err
}

const (
	info   = `{"HostName":"bench","Version":"10.4"}`
	idle   = `{"status":0,"status_name":"idle","uptimeTotalSeconds":100}`
	fresh  = `{"status":0,"status_name":"idle","uptimeTotalSeconds":3}`
	busy   = `{"status":1,"status_name":"playing"}`
	answer = `{"status":0}`
)

func newRun(t *testing.T, api *fake.API, sections ...section.Section) (*Run, *bytes.Buffer) {
	t.Helper()
	api.Reply("GET", "/api/system/info", 200, info)
	out := &bytes.Buffer{}
	return &Run{
		Cfg:      &config.Controller{Version: "9.0"},
		T:        &target.Target{API: api, Address: "192.168.1.20"},
		Sections: sections,
		Opt: Options{
			DiffCommand: []string{"diff", "-u"},
			Out:         out,
			Settle:      5 * time.Second,
		},
	}, out
}

func TestPushOrder(t *testing.T) {
	var log []string
	// Status: the idle check, uptime before the restart, the old fppd still
	// answering once, then the new one.
	api := fake.NewAPI(t).
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/fppd/status", 200, fresh).
		Reply("GET", "/api/system/fppd/restart?quick=1", 200, `{"status":"OK"}`)
	r, out := newRun(t, api,
		&stub{name: "settings", log: &log, after: section.QuickRestart,
			want: state.State{{Key: "settings.fppMode", Value: "remote"}},
			have: state.State{{Key: "settings.fppMode", Value: "player"}}},
		&stub{name: "audio", log: &log, after: section.Reboot,
			want: state.State{{Key: "audio.volume", Value: "70"}},
			have: state.State{{Key: "audio.volume", Value: "70"}}},
		&stub{name: "network", log: &log,
			want: state.State{{Key: "network.eth0.address", Value: "192.168.1.21"}},
			have: state.State{{Key: "network.eth0.address", Value: "192.168.1.20"}}},
	)
	if err := r.Push(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(log, "; ")
	wantLog := "observe settings; observe audio; observe network; apply settings settings.fppMode; apply network network.eth0.address"
	if got != wantLog {
		t.Errorf("log:\n got %s\nwant %s", got, wantLog)
	}
	// The unchanged section's Reboot is never asked for; the quick restart
	// comes before the network.
	var paths []string
	for _, q := range api.Requests() {
		paths = append(paths, q.Path)
	}
	if !strings.HasSuffix(strings.Join(paths, " "), "restart?quick=1 /api/fppd/status /api/fppd/status") {
		t.Errorf("requests: %v", paths)
	}
	for _, s := range []string{"Expected FPP 9.0", "-settings.fppMode = player", "+settings.fppMode = remote", "quick restart"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("output lacks %q:\n%s", s, out)
		}
	}
}

func TestPushRefusesWhilePlaying(t *testing.T) {
	var log []string
	api := fake.NewAPI(t).Reply("GET", "/api/fppd/status", 200, busy)
	r, _ := newRun(t, api, &stub{name: "settings", log: &log})
	err := r.Push(context.Background())
	if err == nil || !strings.Contains(err.Error(), "playing") {
		t.Fatalf("err = %v", err)
	}
	if len(log) != 0 {
		t.Errorf("sections touched: %v", log)
	}
}

func TestPushUnreadableStatusIsNotIdle(t *testing.T) {
	api := fake.NewAPI(t).Reply("GET", "/api/fppd/status", 503, "<html>")
	r, _ := newRun(t, api)
	if err := r.Push(context.Background()); err == nil {
		t.Fatal("pushed with an unreadable status")
	}
}

func TestPushForceSkipsStatus(t *testing.T) {
	var log []string
	api := fake.NewAPI(t)
	r, _ := newRun(t, api, &stub{name: "audio", log: &log,
		want: state.State{{Key: "audio.volume", Value: "5"}}, have: state.State{{Key: "audio.volume", Value: "5"}}})
	r.Opt.Force = true
	if err := r.Push(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPushStopsOnError(t *testing.T) {
	var log []string
	api := fake.NewAPI(t).Reply("GET", "/api/fppd/status", 200, idle)
	ch := state.State{{Key: "k", Value: "a"}}
	r, _ := newRun(t, api,
		&stub{name: "keys", log: &log, want: ch, err: errors.New("boom")},
		&stub{name: "network", log: &log, want: ch})
	err := r.Push(context.Background())
	if err == nil || !strings.Contains(err.Error(), "keys: boom") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(strings.Join(log, ";"), "apply network") {
		t.Errorf("network applied after an error: %v", log)
	}
}

func TestDiffNoChanges(t *testing.T) {
	var log []string
	s := state.State{{Key: "audio.volume", Value: "70"}}
	r, out := newRun(t, fake.NewAPI(t), &stub{name: "audio", log: &log, want: s, have: s},
		&stub{name: "unmanaged", log: &log})
	code, err := r.Diff(context.Background())
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "No differences.") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(strings.Join(log, ";"), "unmanaged") {
		t.Errorf("observed a section the config does not manage: %v", log)
	}
}

// A reboot waits until the network has been applied: booting brings up the
// stored network config.
func TestPushRebootAfterNetwork(t *testing.T) {
	var log []string
	api := fake.NewAPI(t).
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/fppd/status", 200, fresh).
		Reply("GET", "/api/system/reboot", 200, `{"status":"OK"}`)
	ch := func(k string) (state.State, state.State) {
		return state.State{{Key: k, Value: "new"}}, state.State{{Key: k, Value: "old"}}
	}
	w1, h1 := ch("settings.HostName")
	w2, h2 := ch("network.eth0.address")
	r, _ := newRun(t, api,
		&stub{name: "settings", log: &log, after: section.Reboot, want: w1, have: h1},
		&stub{name: "network", log: &log, want: w2, have: h2},
	)
	r.Opt.RebootPause = time.Millisecond
	if err := r.Push(context.Background()); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, l := range log {
		if strings.HasPrefix(l, "apply") {
			order = append(order, l)
		}
	}
	var paths []string
	for _, q := range api.Requests() {
		paths = append(paths, q.Path)
	}
	if !strings.HasSuffix(strings.Join(paths, " "), "/api/system/reboot /api/fppd/status") {
		t.Errorf("requests: %v", paths)
	}
	if len(order) != 2 || !strings.Contains(order[1], "network") {
		t.Errorf("order %v", order)
	}
}

// A failure after a section asked for a reboot says the reboot was not done.
func TestPushFailureReportsPendingReboot(t *testing.T) {
	var log []string
	api := fake.NewAPI(t).Reply("GET", "/api/fppd/status", 200, idle)
	w1 := state.State{{Key: "settings.HostName", Value: "new"}}
	w2 := state.State{{Key: "network.dns", Value: "new"}}
	r, _ := newRun(t, api,
		&stub{name: "settings", log: &log, after: section.Reboot, want: w1},
		&stub{name: "network", log: &log, want: w2, err: errors.New("verify timed out")},
	)
	err := r.Push(context.Background())
	if err == nil || !strings.Contains(err.Error(), "network: verify timed out") ||
		!strings.Contains(err.Error(), "still need a "+section.Reboot.String()) {
		t.Fatalf("err = %v", err)
	}
	for _, q := range api.Requests() {
		if q.Path == "/api/system/reboot" {
			t.Error("rebooted after the network failed")
		}
	}
}

// An fppd that had run only three seconds is still told apart from the one
// after a restart that took longer than that, by its start time.
func TestSettleYoungFppd(t *testing.T) {
	const (
		young = `{"status":0,"uptimeTotalSeconds":3,"systemUptimeTotalSeconds":1000}`
		still = `{"status":0,"uptimeTotalSeconds":5,"systemUptimeTotalSeconds":1002}`
		newer = `{"status":0,"uptimeTotalSeconds":4,"systemUptimeTotalSeconds":1008}`
	)
	for _, tc := range []struct {
		name  string
		after string
		ok    bool
	}{
		{"old fppd still answering", still, false},
		{"new fppd up after five seconds", newer, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := fake.NewAPI(t).
				Reply("GET", "/api/fppd/status", 200, young).
				Reply("GET", "/api/fppd/status", 200, tc.after).
				Reply("GET", "/api/system/fppd/restart", 200, `{"status":"OK"}`)
			r, _ := newRun(t, api)
			r.Opt.Settle = 3 * time.Second
			if err := r.settle(context.Background(), section.Restart); (err == nil) != tc.ok {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestFppdClockNewer(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	old := fppdClock{uptime: 100, system: f(1000)}
	for _, tc := range []struct {
		name string
		c    fppdClock
		want bool
	}{
		{"lower uptime", fppdClock{uptime: 2}, true},
		{"same fppd, rounded", fppdClock{uptime: 110, system: f(1011)}, false},
		{"later start", fppdClock{uptime: 110, system: f(1012)}, true},
		{"rebooted", fppdClock{uptime: 200, system: f(30)}, true},
		{"no board uptime", fppdClock{uptime: 110}, false},
	} {
		if got := tc.c.newer(old); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

// With fppd down before the request, whatever fppd answers after it is new.
func TestSettleFppdDown(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("GET", "/api/fppd/status", 200, answer).
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/system/fppd/restart", 200, `{"status":"OK"}`)
	r, _ := newRun(t, api)
	r.Opt.Settle = 3 * time.Second
	if err := r.settle(context.Background(), section.Restart); err != nil {
		t.Fatal(err)
	}
}

// A section that fails partway still reports the reboot its earlier writes
// need.
func TestPushPartialFailureReportsReboot(t *testing.T) {
	var log []string
	api := fake.NewAPI(t).Reply("GET", "/api/fppd/status", 200, idle)
	ch := state.State{{Key: "settings.HostName", Value: "new"}}
	r, _ := newRun(t, api,
		&stub{name: "settings", log: &log, after: section.Reboot, want: ch, err: errors.New("writing setting ntpServer: refused")},
		&stub{name: "network", log: &log, want: ch})
	err := r.Push(context.Background())
	if err == nil || !strings.Contains(err.Error(), "still need a "+section.Reboot.String()) {
		t.Fatalf("err = %v", err)
	}
}

// A status that fails once is not taken for a stopped fppd: settle keeps
// reading for Baseline, and then needs a newer fppd.
func TestSettleBaselineRetry(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("GET", "/api/fppd/status", 503, "").
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/system/fppd/restart?quick=1", 200, `{"status":"OK"}`)
	r, _ := newRun(t, api)
	r.Opt.Baseline = 3 * time.Second
	r.Opt.Settle = 3 * time.Second
	if err := r.settle(context.Background(), section.QuickRestart); err == nil {
		t.Fatal("settled on the old fppd after a failed first read")
	}
}

func TestSettleNeedsNewFppd(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("GET", "/api/fppd/status", 200, idle).
		Reply("GET", "/api/system/fppd/restart?quick=1", 200, `{"status":"OK"}`)
	r, _ := newRun(t, api)
	r.Opt.Settle = 3 * time.Second
	if err := r.settle(context.Background(), section.QuickRestart); err == nil {
		t.Fatal("settled while the old fppd was still answering")
	}
}

// diff's exit status is its diff program's.
func TestDiffExitStatus(t *testing.T) {
	ch := func() *stub {
		return &stub{name: "audio", log: new([]string),
			want: state.State{{Key: "audio.volume", Value: "60"}},
			have: state.State{{Key: "audio.volume", Value: "70"}}}
	}
	for _, tc := range []struct {
		cmd  []string
		want int
	}{
		{[]string{"diff", "-u"}, 1},
		{[]string{"sh", "-c", "exit 3", "sh"}, 3},
		{[]string{"true"}, 0},
	} {
		r, _ := newRun(t, fake.NewAPI(t), ch())
		r.Opt.DiffCommand = tc.cmd
		code, err := r.Diff(context.Background())
		if err != nil || code != tc.want {
			t.Errorf("%v: code=%d err=%v, want %d", tc.cmd, code, err, tc.want)
		}
	}
}

// A push whose diff program fails does not apply anything.
func TestPushStopsWhenDiffFails(t *testing.T) {
	var log []string
	api := fake.NewAPI(t).Reply("GET", "/api/fppd/status", 200, idle)
	r, _ := newRun(t, api, &stub{name: "audio", log: &log,
		want: state.State{{Key: "audio.volume", Value: "60"}},
		have: state.State{{Key: "audio.volume", Value: "70"}}})
	r.Opt.DiffCommand = []string{"sh", "-c", "exit 2", "sh"}
	if err := r.Push(context.Background()); err == nil {
		t.Fatal("pushed after the diff program failed")
	}
	if strings.Contains(strings.Join(log, ";"), "apply") {
		t.Errorf("applied: %v", log)
	}
}

// rewriter is a stub that push applies on every run.
type rewriter struct{ stub }

func (*rewriter) Rewrites() bool { return true }

// A rewriter is applied even when diff shows nothing, and never printed.
func TestPushRewrites(t *testing.T) {
	var log []string
	same := state.State{{Key: "passwords.ui.enabled", Value: "1"}}
	api := fake.NewAPI(t).Reply("GET", "/api/fppd/status", 200, idle)
	r, out := newRun(t, api,
		&rewriter{stub{name: "passwords", log: &log, want: same, have: same}},
		&stub{name: "network", log: &log})
	if err := r.Push(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(log, "; ")
	if got != "observe passwords; apply passwords " {
		t.Errorf("log: %s", got)
	}
	if strings.Contains(out.String(), "Nothing to change") {
		t.Errorf("output:\n%s", out)
	}
}
