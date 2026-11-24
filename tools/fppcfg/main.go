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

// Command fppcfg brings one FPP controller to the state in one config, and
// shows the difference first.
//
//	fppcfg ./station.cfg diff [--full] [--at ADDRESS] [--password OLD]
//	fppcfg ./station.cfg push [--force] [--at ADDRESS] [--password OLD]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/engine"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fpp"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/section"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

const usage = `usage:
  fppcfg CONFIG diff [--full] [--at ADDRESS] [--password OLD]
  fppcfg CONFIG push [--force] [--at ADDRESS] [--password OLD]
`

func main() {
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "fppcfg:", err)
		os.Exit(2) // as diff does: 1 means differences
	}
	os.Exit(code)
}

// run returns the exit status: diff's is its diff program's.
func run(args []string) (int, error) {
	if len(args) < 2 {
		return 0, errors.New(strings.TrimSpace(usage))
	}
	path, verb := args[0], args[1]

	fs := flag.NewFlagSet("fppcfg "+verb, flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	at := fs.String("at", "", "reach the controller at `ADDRESS` for this run")
	old := fs.String("password", "", "the controller's current UI password, for the run that changes it")
	var full, force *bool
	switch verb {
	case "diff":
		full = fs.Bool("full", false, "also list managed settings the config leaves unset")
	case "push":
		force = fs.Bool("force", false, "push even while the controller is playing")
	default:
		return 0, fmt.Errorf("unknown command %q\n%s", verb, usage)
	}
	if err := fs.Parse(args[2:]); err != nil {
		return 0, err
	}
	if fs.NArg() > 0 {
		return 0, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	cfg, err := config.Load(path)
	if err != nil {
		return 0, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	addr, err := address(*at, cfg)
	if err != nil {
		return 0, err
	}
	if addr, err = newZoner().withZone(ctx, addr); err != nil {
		return 0, err
	}

	var passwords []string
	if cfg.UIPassword != "" {
		passwords = append(passwords, cfg.UIPassword)
	}
	if *old != "" {
		passwords = append(passwords, *old)
	}
	t := &target.Target{
		API:     fpp.New(addr, passwords),
		Address: addr,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
	r := &engine.Run{
		Cfg:      cfg,
		T:        t,
		Sections: section.All(cfg),
		Opt: engine.Options{
			DiffCommand: diffCommand(),
			Out:         os.Stdout,
			Settle:      3 * time.Minute,
			RebootPause: 10 * time.Second,
			Baseline:    10 * time.Second,
		},
	}

	if verb == "diff" {
		r.Opt.Full = *full
		return r.Diff(ctx)
	}
	r.Opt.Force = *force
	return 0, r.Push(ctx)
}

// address picks where to reach the controller: --at, then the config's
// management address, then the static IPv4 address of its first interface
// (eth0 before others, since the list is sorted).
func address(at string, cfg *config.Controller) (string, error) {
	if at != "" {
		return at, nil
	}
	if cfg.Address != "" {
		return cfg.Address, nil
	}
	for _, i := range cfg.Network.Interfaces {
		if i.Proto == "static" && i.Address != "" {
			return i.Address, nil
		}
	}
	return "", errors.New("the config gives no address and no static interface; use --at")
}

// diffCommand is FPPCFG_DIFF, then DIFF, then "diff -u".
func diffCommand() []string {
	for _, v := range []string{"FPPCFG_DIFF", "DIFF"} {
		if f := strings.Fields(os.Getenv(v)); len(f) > 0 {
			return f
		}
	}
	return []string{"diff", "-u"}
}
