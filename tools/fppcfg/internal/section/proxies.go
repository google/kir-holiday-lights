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
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// proxiesSection is the list of hosts FPP proxies at /proxy/<host>/, written
// whole. Entries: "proxies.count", then "proxies[i]" as the host, followed
// by the quoted description when there is one.
//
// FPP adds a proxy of its own for each address its DHCP server has leased,
// and keeps it in the stored list once it writes that list. Those are left
// out of the observed list unless the config names the host, and written
// back as they were.
type proxiesSection struct {
	cfg *config.Controller
	// leased is FPP's own stored lease proxies the config does not name,
	// from Observe.
	leased []proxyItem
}

func NewProxies(cfg *config.Controller) Section { return &proxiesSection{cfg: cfg} }

func (s *proxiesSection) Name() string { return "proxies" }

const proxiesPath = "/api/proxies"

// proxyItem is one entry as FPP reads and writes it.
type proxyItem struct {
	Host        string `json:"host"`
	Description string `json:"description"`
	// Only in replies: the host has a lease from this FPP's DHCP server,
	// and pending says the stored list does not have it yet.
	DHCP    bool `json:"dhcp,omitempty"`
	Pending bool `json:"pending,omitempty"`
}

func proxyEntry(host, description string) string {
	if description == "" {
		return host
	}
	return host + " " + strconv.Quote(description)
}

func (s *proxiesSection) Want() state.State {
	if !s.cfg.ManageProxies {
		return nil
	}
	items := make([]string, 0, len(s.cfg.Proxies))
	for _, p := range s.cfg.Proxies {
		items = append(items, proxyEntry(p.Host, p.Description))
	}
	return state.List("proxies", items)
}

func (s *proxiesSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	var reply *[]*proxyItem
	if err := target.GetJSON(ctx, t.API, proxiesPath, &reply); err != nil {
		return nil, fmt.Errorf("reading the proxies: %w", err)
	}
	if reply == nil {
		return nil, errors.New("reading the proxies: no list in the reply")
	}
	named := make(map[string]bool, len(s.cfg.Proxies))
	for _, p := range s.cfg.Proxies {
		named[p.Host] = true
	}
	items := []string{}
	s.leased = nil
	for i, p := range *reply {
		if p == nil || p.Host == "" {
			return nil, fmt.Errorf("reading the proxies: item %d has no host", i)
		}
		if p.Pending {
			continue
		}
		if p.DHCP && !named[p.Host] {
			s.leased = append(s.leased, proxyItem{Host: p.Host, Description: p.Description})
			continue
		}
		items = append(items, proxyEntry(p.Host, p.Description))
	}
	return state.List("proxies", items), nil
}

// Apply replaces the whole list, keeping FPP's own lease proxies. FPP
// refuses an empty list, so no proxies at all is a DELETE, which FPP 8.x
// does not have. FPP reloads Apache itself.
func (s *proxiesSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	items := make([]proxyItem, 0, len(s.cfg.Proxies)+len(s.leased))
	for _, p := range s.cfg.Proxies {
		items = append(items, proxyItem{Host: p.Host, Description: p.Description})
	}
	items = append(items, s.leased...)
	if len(items) == 0 {
		resp, err := t.API.Do(ctx, http.MethodDelete, proxiesPath, nil, "")
		if err == nil && !resp.OK() {
			err = &target.StatusError{Method: http.MethodDelete, Path: proxiesPath, Status: resp.Status}
		}
		if err != nil {
			return None, fmt.Errorf("removing the proxies: %w", err)
		}
		return None, nil
	}
	if err := target.PostJSON(ctx, t.API, proxiesPath, items); err != nil {
		return None, fmt.Errorf("saving the proxies: %w", err)
	}
	return None, nil
}
