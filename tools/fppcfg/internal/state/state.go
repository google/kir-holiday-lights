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

// Package state is the one representation fppcfg compares: what the config
// wants and what the controller has, both as flat key/value entries.
//
// diff renders the two sides to text for an external diff program; push
// compares them once and hands each section exactly the changes it printed.
package state

import (
	"fmt"
	"strings"
)

// Entry is one fact: a key such as "settings.fppMode" or "keys[0]", and its
// value as plain text. Nothing secret is ever a value: a section that
// manages a secret reports only whether it is set.
type Entry struct {
	Key   string
	Value string
}

// State is a section's entries in the order the section emits them.
type State []Entry

// Get returns the value for key and whether it is present.
func (s State) Get(key string) (string, bool) {
	for _, e := range s {
		if e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}

// List is a whole list as entries: "<prefix>.count", then "<prefix>[i]" for
// each item, so order, duplicates and an empty list all show.
func List(prefix string, items []string) State {
	s := State{{Key: prefix + ".count", Value: fmt.Sprint(len(items))}}
	for i, item := range items {
		s = append(s, Entry{Key: fmt.Sprintf("%s[%d]", prefix, i), Value: item})
	}
	return s
}

// Change is one key whose wanted and observed values differ. Want is nil
// when the controller has an entry the config does not (an extra list item);
// Have is nil when the controller has no entry for the key.
type Change struct {
	Key  string
	Want *string
	Have *string
}

// Compare returns the changes between want and have: every key of want
// whose value differs from have's or is missing there, then every key of
// have that want lacks. Order follows want, then have.
func Compare(want, have State) []Change {
	var out []Change
	seen := make(map[string]bool, len(want))
	for _, w := range want {
		seen[w.Key] = true
		h, ok := have.Get(w.Key)
		switch {
		case !ok:
			out = append(out, Change{Key: w.Key, Want: &w.Value})
		case w.Value != h:
			out = append(out, Change{Key: w.Key, Want: &w.Value, Have: &h})
		}
	}
	for _, h := range have {
		if !seen[h.Key] {
			out = append(out, Change{Key: h.Key, Have: &h.Value})
		}
	}
	return out
}

// Changed is the set of keys in changes.
func Changed(changes []Change) map[string]bool {
	out := make(map[string]bool, len(changes))
	for _, c := range changes {
		out[c.Key] = true
	}
	return out
}

// Render writes state as "key = value" lines for the diff program.
func Render(s State) string {
	var b strings.Builder
	for _, e := range s {
		fmt.Fprintf(&b, "%s = %s\n", e.Key, e.Value)
	}
	return b.String()
}
