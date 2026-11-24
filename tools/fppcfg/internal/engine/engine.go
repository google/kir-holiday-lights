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

// Package engine runs diff and push over the sections, in the order
// docs/design.md gives: check, observe and compare once, print, apply the
// sections, restart fppd once, apply the network, then reboot once.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/section"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// Options are the command-line choices that shape a run.
type Options struct {
	Full  bool // diff --full
	Force bool // push --force
	// DiffCommand is the diff program and its arguments; the two file names
	// are appended.
	DiffCommand []string
	Out         io.Writer
	// Settle is how long to wait for the controller after a restart or
	// reboot. Tests shorten it.
	Settle time.Duration
	// RebootPause is how long to let a rebooting board go down before
	// looking for it.
	RebootPause time.Duration
	// Baseline is how long to keep reading fppd's status before a restart
	// or reboot. When it never answers, fppd is taken to be down.
	Baseline time.Duration
}

// Run is one controller, one run.
type Run struct {
	Cfg      *config.Controller
	T        *target.Target
	Sections []section.Section
	Opt      Options
}

// planned is one section with what was printed for it: its wanted and
// observed state and the changes between them.
type planned struct {
	section    section.Section
	want, have state.State
	changes    []state.Change
}

// plan is every section, in push order.
type plan []planned

func (p plan) empty() bool {
	for _, s := range p {
		if len(s.changes) > 0 {
			return false
		}
	}
	return true
}

// Diff prints the difference and writes nothing. It returns the diff
// program's exit status (0 when there is nothing to compare, so the program
// is not run).
func (r *Run) Diff(ctx context.Context) (int, error) {
	if err := r.identify(ctx); err != nil {
		return 0, err
	}
	p, err := r.observe(ctx)
	if err != nil {
		return 0, err
	}
	code, err := r.print(p)
	if err != nil {
		return 0, err
	}
	if p.empty() {
		fmt.Fprintln(r.Opt.Out, "No differences.")
	}
	if r.Opt.Full {
		if err := r.printUnset(ctx); err != nil {
			return 0, err
		}
	}
	return code, nil
}

// Push prints the difference and applies exactly it.
func (r *Run) Push(ctx context.Context) error {
	if err := r.identify(ctx); err != nil {
		return err
	}
	if err := r.checkIdle(ctx); err != nil {
		return err
	}
	p, err := r.observe(ctx)
	if err != nil {
		return err
	}
	code, err := r.print(p)
	if err != nil {
		return err
	}
	if code > 1 {
		return fmt.Errorf("%s exited with status %d", r.Opt.DiffCommand[0], code)
	}
	if p.empty() && !p.rewrites() {
		fmt.Fprintln(r.Opt.Out, "Nothing to change.")
		return nil
	}

	last := len(p) - 1 // network
	after := section.None
	for _, s := range p[:last] {
		a, err := r.apply(ctx, s)
		if err != nil {
			return pending(err, max(after, a))
		}
		after = max(after, a)
	}
	// An fppd restart comes before the network. A reboot waits until after
	// it: booting brings up whatever network config is stored, so the
	// network must be applied and confirmed first, or the board could come
	// back at an address the tool does not know.
	if after < section.Reboot {
		if err := r.settle(ctx, after); err != nil {
			return err
		}
		after = section.None
	}
	a, err := r.apply(ctx, p[last])
	if err != nil {
		return pending(err, max(after, a))
	}
	if err := r.settle(ctx, max(after, a)); err != nil {
		return err
	}
	fmt.Fprintln(r.Opt.Out, "Done.")
	return nil
}

// pending adds to err that changes already written still need a restart or
// reboot, which the failure stopped the run from doing.
func pending(err error, after section.After) error {
	if after == section.None {
		return err
	}
	return fmt.Errorf("%w; changes already written still need a %s, which was not done", err, after)
}

// rewrites reports whether a section writes on every push.
func (p plan) rewrites() bool {
	for _, s := range p {
		if rewrites(s.section) {
			return true
		}
	}
	return false
}

func rewrites(s section.Section) bool {
	rw, ok := s.(section.Rewriter)
	return ok && rw.Rewrites()
}

func (r *Run) apply(ctx context.Context, s planned) (section.After, error) {
	if len(s.changes) == 0 && !rewrites(s.section) {
		return section.None, nil
	}
	fmt.Fprintf(r.Opt.Out, "Applying %s.\n", s.section.Name())
	a, err := s.section.Apply(ctx, r.T, s.changes)
	if err != nil {
		// a still counts: what was written before the failure needs it.
		return a, fmt.Errorf("%s: %w", s.section.Name(), err)
	}
	return a, nil
}

// identify reads the controller's version, and reports
// a version other than the one the config expects.
func (r *Run) identify(ctx context.Context) error {
	var info struct {
		Version  string `json:"Version"`
		HostName string `json:"HostName"`
	}
	if err := target.GetJSON(ctx, r.T.API, "/api/system/info", &info); err != nil {
		return fmt.Errorf("reading the controller at %s: %w", r.T.Address, err)
	}
	fmt.Fprintf(r.Opt.Out, "%s at %s, FPP %s.\n", info.HostName, r.T.Address, info.Version)
	if v := r.Cfg.Version; v != "" && v != info.Version {
		fmt.Fprintf(r.Opt.Out, "Expected FPP %s; the controller runs %s.\n", v, info.Version)
	}
	return nil
}

// checkIdle refuses to push while the controller plays, unless forced. A
// status that cannot be read is not idle.
func (r *Run) checkIdle(ctx context.Context) error {
	if r.Opt.Force {
		return nil
	}
	var st struct {
		Status     *int   `json:"status"`
		StatusName string `json:"status_name"`
	}
	if err := target.GetJSON(ctx, r.T.API, "/api/fppd/status", &st); err != nil {
		return fmt.Errorf("cannot tell whether the controller is playing (use --force to push anyway): %w", err)
	}
	if st.Status == nil {
		return errors.New("cannot tell whether the controller is playing: no status in the reply (use --force to push anyway)")
	}
	if *st.Status != 0 {
		return fmt.Errorf("the controller is %s; not pushing (use --force to push anyway)", st.StatusName)
	}
	return nil
}

func (r *Run) observe(ctx context.Context) (plan, error) {
	var p plan
	for _, s := range r.Sections {
		want := s.Want()
		var have state.State
		if len(want) > 0 {
			var err error
			have, err = s.Observe(ctx, r.T)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.Name(), err)
			}
		}
		p = append(p, planned{s, want, have, state.Compare(want, have)})
		if n, ok := s.(section.Notes); ok {
			for _, note := range n.Notes() {
				fmt.Fprintf(r.Opt.Out, "%s: %s\n", s.Name(), note)
			}
		}
	}
	return p, nil
}

// print hands the observed and wanted state to the diff program as two
// files, so colordiff, delta and the like work. It returns the program's
// exit status; err is for a program that could not be run at all.
func (r *Run) print(p plan) (int, error) {
	if p.empty() {
		return 0, nil
	}
	var have, want strings.Builder
	for _, s := range p {
		have.WriteString(state.Render(s.have))
		want.WriteString(state.Render(s.want))
	}
	dir, err := os.MkdirTemp("", "fppcfg-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	cur := filepath.Join(dir, "controller")
	cfg := filepath.Join(dir, "config")
	if err := os.WriteFile(cur, []byte(have.String()), 0o600); err != nil {
		return 0, err
	}
	if err := os.WriteFile(cfg, []byte(want.String()), 0o600); err != nil {
		return 0, err
	}
	args := append(r.Opt.DiffCommand[1:len(r.Opt.DiffCommand):len(r.Opt.DiffCommand)], cur, cfg)
	cmd := exec.Command(r.Opt.DiffCommand[0], args...)
	cmd.Dir = dir
	cmd.Stdout = r.Opt.Out
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("running %s: %w", r.Opt.DiffCommand[0], err)
	}
	return 0, nil
}

func (r *Run) printUnset(ctx context.Context) error {
	for _, s := range r.Sections {
		u, ok := s.(section.Unsetter)
		if !ok {
			continue
		}
		st, err := u.Unset(ctx, r.T)
		if err != nil {
			return fmt.Errorf("%s: %w", s.Name(), err)
		}
		if len(st) == 0 {
			continue
		}
		fmt.Fprintln(r.Opt.Out, "Not set by the config:")
		fmt.Fprint(r.Opt.Out, indent(state.Render(st)))
	}
	return nil
}

func indent(s string) string {
	lines := strings.SplitAfter(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "")
}

// settle restarts fppd or reboots, as after says, and waits until a new
// fppd answers (fppdClock.newer). A quick restart answers at once and the
// old fppd can still answer status for a moment, so "status answers" alone
// would pass too early. When fppd does not answer for Baseline before the
// request, it is taken to be down: there is no old fppd to mistake for the
// new one, and any fppd that answers will do.
func (r *Run) settle(ctx context.Context, after section.After) error {
	var path string
	switch after {
	case section.None:
		return nil
	case section.QuickRestart:
		path = "/api/system/fppd/restart?quick=1"
	case section.Restart:
		path = "/api/system/fppd/restart"
	case section.Reboot:
		path = "/api/system/reboot"
	}
	fmt.Fprintf(r.Opt.Out, "Changes need a %s.\n", after)
	before, ok := readFppdClock(ctx, r.T.API)
	if !ok {
		base, cancel := context.WithTimeout(ctx, r.Opt.Baseline)
		target.WaitFor(base, func(ctx context.Context) bool {
			before, ok = readFppdClock(ctx, r.T.API)
			return ok
		}, 2*time.Second)
		cancel()
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if !ok {
		before = fppdClock{uptime: math.Inf(1)}
	}
	resp, err := r.T.API.Do(ctx, "GET", path, nil, "")
	if err != nil {
		return fmt.Errorf("asking for a %s: %w", after, err)
	}
	if !resp.OK() {
		return fmt.Errorf("asking for a %s: HTTP %d", after, resp.Status)
	}
	if after == section.Reboot {
		// Let it go down before looking for it to come back.
		select {
		case <-time.After(r.Opt.RebootPause):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	wait, cancel := context.WithTimeout(ctx, r.Opt.Settle)
	defer cancel()
	err = target.WaitFor(wait, func(ctx context.Context) bool {
		now, ok := readFppdClock(ctx, r.T.API)
		return ok && now.newer(before)
	}, 2*time.Second)
	if err != nil {
		return fmt.Errorf("the controller did not come back after a %s: %w", after, err)
	}
	return nil
}

// fppdClock is what fppd's status says about when it started.
type fppdClock struct {
	uptime float64 // fppd's, in seconds
	// system is the board's uptime in seconds, from the same reply, or nil
	// when the status does not give it.
	system *float64
}

// newer reports whether c comes from a later fppd than old. A lower uptime
// shows it, but only while the new fppd answers sooner than the old one had
// run: an old fppd started three seconds before a restart that takes five
// never shows one. So the start time, the board's uptime less fppd's, counts
// too. Both are whole seconds, so the old fppd's start time can seem to move
// by up to two; a later one must be ahead by at least that. A lower board
// uptime is a reboot.
func (c fppdClock) newer(old fppdClock) bool {
	if c.uptime < old.uptime {
		return true
	}
	if c.system == nil || old.system == nil {
		return false
	}
	if *c.system < *old.system {
		return true
	}
	return *c.system-c.uptime >= *old.system-old.uptime+2
}

// readFppdClock reads fppd's status. ok is false while fppd does not answer
// properly.
func readFppdClock(ctx context.Context, api target.API) (fppdClock, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := api.Do(ctx, "GET", "/api/fppd/status", nil, "")
	if err != nil || !resp.OK() {
		return fppdClock{}, false
	}
	var v struct {
		Uptime *float64 `json:"uptimeTotalSeconds"`
		System *float64 `json:"systemUptimeTotalSeconds"`
	}
	if json.Unmarshal(resp.Body, &v) != nil || v.Uptime == nil {
		return fppdClock{}, false
	}
	return fppdClock{uptime: *v.Uptime, system: v.System}, true
}
