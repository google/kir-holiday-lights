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

package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

//go:embed schema.cue
var schemaSource []byte

const schemaName = "schema.cue"

// setupOrder is the order of Controller.Setup, as config.go promises.
var setupOrder = []string{
	"LegalJurisdiction",
	"statsPublish",
	"ShareCrashData",
	"FetchVendorLogos",
	"SendVendorSerial",
}

// Load reads the config at path together with every .cue file in the same
// directory, checks the result against the schema and returns it.
func Load(path string) (*Controller, error) {
	ctx := cuecontext.New()
	schema, err := compileSchema(ctx)
	if err != nil {
		return nil, err
	}
	files, err := configFiles(path)
	if err != nil {
		return nil, err
	}
	user, err := buildConfig(ctx, files)
	if err != nil {
		return nil, err
	}
	// Unifying in this order keeps "#Controller" out of error paths.
	v := user.Unify(schema.LookupPath(cue.ParsePath("#Controller")))
	if err := v.Validate(cue.Concrete(true)); err != nil {
		return nil, cueError(err, user)
	}
	if err := checkPSKHasSSID(user); err != nil {
		return nil, err
	}
	if err := checkNetworkHasInterfaces(user); err != nil {
		return nil, err
	}
	return decode(v)
}

// ManagedSettings lists every ordinary setting the schema allows, in schema
// order.
func ManagedSettings() []string {
	return slices.Clone(managedSettings())
}

var managedSettings = sync.OnceValue(func() []string {
	schema, err := compileSchema(cuecontext.New())
	if err != nil {
		panic(err) // the embedded schema is fixed at build time
	}
	return fieldNames(schema.LookupPath(cue.ParsePath("#Settings")))
})

func compileSchema(ctx *cue.Context) (cue.Value, error) {
	v := ctx.CompileBytes(schemaSource, cue.Filename(schemaName))
	if err := v.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("config schema: %w", err)
	}
	return v, nil
}

// configFiles returns path followed by the .cue files beside it. A .cue
// path is not listed twice.
func configFiles(path string) ([]string, error) {
	dir := filepath.Dir(path)
	others, err := filepath.Glob(filepath.Join(dir, "*.cue"))
	if err != nil {
		return nil, err
	}
	self, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	files := []string{path}
	for _, f := range others {
		fi, err := os.Stat(f)
		if err != nil {
			return nil, err
		}
		if fi.Mode().IsRegular() && !os.SameFile(fi, self) {
			files = append(files, f)
		}
	}
	return files, nil
}

// buildConfig parses the files as one CUE instance. As in CUE generally, a
// file sees another's fields (a hidden _wifiPSK in secrets.cue, say) only
// when both have the same package clause; files with no clause are just
// unified. Two different package names are an error.
func buildConfig(ctx *cue.Context, files []string) (cue.Value, error) {
	inst := build.NewContext().NewInstance(filepath.Dir(files[0]), nil)
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			return cue.Value{}, err
		}
		f, err := parser.ParseFile(name, src, parser.ParseComments)
		if err != nil {
			return cue.Value{}, cueError(err, cue.Value{})
		}
		if err := inst.AddSyntax(f); err != nil {
			return cue.Value{}, cueError(err, cue.Value{})
		}
	}
	v := ctx.BuildInstance(inst)
	if err := v.Err(); err != nil {
		return cue.Value{}, cueError(err, cue.Value{})
	}
	return v, nil
}

// checkPSKHasSSID is done here rather than in the schema because CUE has no
// plain way to make one optional field depend on another being present. It
// looks at the config before the schema, so the position is in a config file.
func checkPSKHasSSID(user cue.Value) error {
	ifaces := user.LookupPath(cue.ParsePath("network.interfaces"))
	if !ifaces.Exists() {
		return nil
	}
	it, err := ifaces.Fields()
	if err != nil {
		return cueError(err, cue.Value{})
	}
	for it.Next() {
		psk := it.Value().LookupPath(cue.ParsePath("psk"))
		ssid := it.Value().LookupPath(cue.ParsePath("ssid"))
		if psk.Exists() && !ssid.Exists() {
			return cueError(cueerrors.Newf(psk.Pos(),
				"network.interfaces.%s.psk: set without ssid", it.Selector()), cue.Value{})
		}
	}
	return nil
}

// checkNetworkHasInterfaces rejects network.dns or network.gateway without
// an interface: FPP applies them only when it applies an interface, so the
// tool could set them but never make them take effect.
func checkNetworkHasInterfaces(user cue.Value) error {
	nw := user.LookupPath(cue.ParsePath("network"))
	if !nw.Exists() {
		return nil
	}
	ifaces := nw.LookupPath(cue.ParsePath("interfaces"))
	if ifaces.Exists() {
		it, err := ifaces.Fields()
		if err != nil {
			return cueError(err, cue.Value{})
		}
		for it.Next() {
			if isSet(it.Value()) {
				return nil
			}
		}
	}
	var list cueerrors.Error
	for _, name := range []string{"dns", "gateway"} {
		if f := nw.LookupPath(cue.ParsePath(name)); isSet(f) {
			list = cueerrors.Append(list, cueerrors.Newf(f.Pos(),
				"network.%s: set without network.interfaces; it takes effect only when an interface is applied", name))
		}
	}
	if list != nil {
		return cueError(list, cue.Value{})
	}
	return nil
}

// decode reads the validated config into a Controller. The schema has
// already checked every value, so an error here is a bug in the decoding,
// not in the config.
func decode(v cue.Value) (*Controller, error) {
	c := &Controller{
		Version:    stringAt(v, "version"),
		Address:    stringAt(v, "address"),
		UIPassword: stringAt(v, "passwords.ui"),
		OSPassword: stringAt(v, "passwords.os"),
	}
	if vol := at(v, "volume"); isSet(vol) {
		n, _ := vol.Int64()
		i := int(n)
		c.Volume = &i
	}
	field := func(name string) cue.Value { return v.LookupPath(cue.ParsePath(name)) }
	var errs [6]error
	c.Settings, errs[0] = settingsAt(field("settings"), managedSettings())
	c.Setup, errs[1] = settingsAt(field("setup"), setupOrder)
	c.SSHKeys, c.ManageKeys, errs[2] = listAt(field("sshKeys"), stringItem)
	c.OutputProcessors, c.ManageProcessors, errs[3] = listAt(field("outputProcessors"), processorItem)
	c.Proxies, c.ManageProxies, errs[4] = proxiesAt(field("proxies"))
	c.Network, errs[5] = network(field("network"))
	if err := errors.Join(errs[:]...); err != nil {
		return nil, fmt.Errorf("config: decoding: %s", hideLiterals(err.Error()))
	}
	return c, nil
}

// at looks up path and resolves a default ("*1 | int") to its value, which
// Kind and the other accessors do not do on their own.
func at(v cue.Value, path string) cue.Value {
	f, _ := v.LookupPath(cue.ParsePath(path)).Default()
	return f
}

// isSet reports whether the config gives v a value. A field that is absent
// or null is not set: null is how a config drops a shared file's default.
func isSet(v cue.Value) bool {
	f, _ := v.Default()
	return f.Exists() && f.Kind() != cue.NullKind
}

// stringAt returns the string at path, or "" when it is absent. The schema
// has already checked the type.
func stringAt(v cue.Value, path string) string {
	s, _ := at(v, path).String()
	return s
}

func boolAt(v cue.Value, path string) bool {
	b, _ := at(v, path).Bool()
	return b
}

// settingsAt returns the fields of v that are named in order, in that
// order, as FPP strings.
func settingsAt(v cue.Value, order []string) ([]Setting, error) {
	var out []Setting
	for _, name := range order {
		f, _ := v.LookupPath(cue.MakePath(cue.Str(name))).Default()
		if !isSet(f) {
			continue
		}
		s, err := fppString(f)
		if err != nil {
			return nil, err
		}
		out = append(out, Setting{Name: name, Value: s})
	}
	return out, nil
}

// fppString formats a value the way FPP stores settings: booleans as "1"
// and "0", numbers plainly.
func fppString(v cue.Value) (string, error) {
	switch v.Kind() {
	case cue.BoolKind:
		b, err := v.Bool()
		if b {
			return "1", err
		}
		return "0", err
	case cue.IntKind:
		n, err := v.Int64()
		return strconv.FormatInt(n, 10), err
	case cue.FloatKind:
		f, err := v.Float64()
		return strconv.FormatFloat(f, 'f', -1, 64), err
	case cue.StringKind:
		return v.String()
	}
	return "", fmt.Errorf("%s: cannot be an FPP setting (kind %v)", v.Path(), v.Kind())
}

// fieldNames lists a struct's fields, optional ones included, in order.
func fieldNames(v cue.Value) []string {
	var names []string
	it, err := v.Fields(cue.Optional(true))
	if err != nil {
		panic(err) // only called on the embedded schema
	}
	for it.Next() {
		names = append(names, it.Selector().Unquoted())
	}
	return names
}

// listAt returns the list at v with each item converted by conv, and
// whether the config sets it. A set, empty list is not nil.
func listAt(v cue.Value, conv func(cue.Value) (string, error)) ([]string, bool, error) {
	if !isSet(v) {
		return nil, false, nil
	}
	v, _ = v.Default()
	out := []string{}
	it, err := v.List()
	if err != nil {
		return nil, true, err
	}
	for it.Next() {
		s, err := conv(it.Value())
		if err != nil {
			return nil, true, err
		}
		out = append(out, s)
	}
	return out, true, nil
}

func stringItem(v cue.Value) (string, error) {
	item, _ := v.Default()
	return item.String()
}

// processorItem renders one output processor as canonical JSON.
func processorItem(v cue.Value) (string, error) {
	raw, err := v.MarshalJSON()
	if err != nil {
		return "", err
	}
	return CanonicalJSON(raw)
}

// CanonicalJSON renders a JSON value with keys sorted and no spaces, so two
// equal values compare equal as strings. Numbers keep their text. It is the
// form of Controller.OutputProcessors.
func CanonicalJSON(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(x); err != nil { // maps encode with sorted keys
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// proxiesAt returns the proxy list at v, and whether the config sets it.
// A set, empty list is not nil.
func proxiesAt(v cue.Value) ([]Proxy, bool, error) {
	if !isSet(v) {
		return nil, false, nil
	}
	v, _ = v.Default()
	out := []Proxy{}
	it, err := v.List()
	if err != nil {
		return nil, true, err
	}
	for it.Next() {
		item := it.Value()
		out = append(out, Proxy{Host: stringAt(item, "host"), Description: stringAt(item, "description")})
	}
	return out, true, nil
}

// fromDHCP is what the config writes for the gateway or DNS to say the
// controller keeps none of its own.
const fromDHCP = "dhcp"

func network(v cue.Value) (Network, error) {
	var n Network
	if !v.Exists() {
		return n, nil
	}
	if gw := at(v, "gateway"); isSet(gw) {
		n.ManageGateway = true
		if n.Gateway, _ = gw.String(); n.Gateway == fromDHCP {
			n.Gateway = ""
		}
	}
	if dns := at(v, "dns"); dns.Kind() == cue.StringKind { // "dhcp"
		n.DNS, n.ManageDNS = []string{}, true
	} else {
		var err error
		n.DNS, n.ManageDNS, err = listAt(dns, stringItem)
		if err != nil {
			return n, err
		}
	}
	ifaces := v.LookupPath(cue.ParsePath("interfaces"))
	if !ifaces.Exists() {
		return n, nil
	}
	it, err := ifaces.Fields()
	if err != nil {
		return n, err
	}
	for it.Next() {
		iv := it.Value()
		if !isSet(iv) {
			continue
		}
		n.Interfaces = append(n.Interfaces, Interface{
			Name:    it.Selector().Unquoted(),
			Proto:   stringAt(iv, "proto"),
			Address: stringAt(iv, "address"),
			Netmask: stringAt(iv, "netmask"),
			SSID:    stringAt(iv, "ssid"),
			PSK:     stringAt(iv, "psk"),
			Hidden:  boolAt(iv, "hidden"),
			WPA3:    boolAt(iv, "wpa3"),
		})
	}
	sort.Slice(n.Interfaces, func(i, j int) bool {
		return n.Interfaces[i].Name < n.Interfaces[j].Name
	})
	return n, nil
}

// cueError formats a CUE error with each problem's field path and file
// positions. user, when it exists, is the config before the schema was
// applied; it locates problems CUE reports only against the schema.
func cueError(err error, user cue.Value) error {
	var list cueerrors.Error
	for _, e := range withoutNullNoise(cueerrors.Errors(err)) {
		list = cueerrors.Append(list, clean(e, user))
	}
	return fmt.Errorf("config: %s", strings.TrimSpace(cueerrors.Details(list, nil)))
}

// withoutNullNoise drops what the schema's "| null" alternatives add to a
// report. A bad value fails both alternatives, so CUE reports the real
// problem, a line that the value is not null, and a header counting the
// two. Only the real problem is kept, field by field.
func withoutNullNoise(errs []cueerrors.Error) []cueerrors.Error {
	noise := func(e cueerrors.Error) bool {
		format, args := e.Msg()
		msg := fmt.Sprintf(format, args...)
		return strings.Contains(msg, "errors in empty disjunction") ||
			strings.Contains(msg, "mismatched types") && strings.Contains(msg, "null") && againstSchema(e)
	}
	// A field keeps its noise when that is all there is to say about it or
	// anything inside it.
	real := map[string]bool{}
	for _, e := range errs {
		if noise(e) {
			continue
		}
		path := e.Path()
		real[strings.Join(path, ".")] = true
		for i := 1; i < len(path); i++ {
			real[strings.Join(path[:i], ".")] = true
		}
	}
	var kept []cueerrors.Error
	for _, e := range errs {
		if !noise(e) || !real[strings.Join(e.Path(), ".")] {
			kept = append(kept, e)
		}
	}
	return kept
}

// againstSchema reports whether e sets one place in the config against the
// schema, rather than two places in the config against each other.
func againstSchema(e cueerrors.Error) bool {
	inSchema, inFiles := false, 0
	for _, pos := range cueerrors.Positions(e) {
		switch {
		case !pos.IsValid():
		case pos.Filename() == schemaName:
			inSchema = true
		default:
			inFiles++
		}
	}
	return inSchema && inFiles <= 1
}

// clean hides string literals in e's message and adds a position in the
// config when e has none.
func clean(e cueerrors.Error, user cue.Value) cueerrors.Error {
	p := problem{inner: e}
	format, args := e.Msg()
	p.msg = hideLiterals(fmt.Sprintf(format, args...))
	if !inConfig(cueerrors.Positions(e)) && user.Exists() {
		if pos := nearestPos(user, e.Path()); pos.IsValid() {
			p.extra = pos
		}
	}
	return p
}

// inConfig reports whether any position is in a config file rather than
// the embedded schema.
func inConfig(positions []token.Pos) bool {
	for _, pos := range positions {
		if pos.IsValid() && pos.Filename() != schemaName {
			return true
		}
	}
	return false
}

// nearestPos returns where the closest existing ancestor of path is
// written in the config, falling back to the config as a whole.
func nearestPos(v cue.Value, path []string) token.Pos {
	for n := len(path); n >= 0; n-- {
		f := v.LookupPath(cue.ParsePath(strings.Join(path[:n], ".")))
		if f.Exists() && f.Pos().IsValid() {
			return f.Pos()
		}
	}
	return token.NoPos
}

// hidden replaces each string or bytes literal in an error message.
const hidden = `"(hidden)"`

// hideLiterals replaces every quoted literal in msg with hidden. A secret is
// often defined once as a hidden field or definition and referenced, so a
// conflict over it is reported at a path that says nothing about secrets;
// hiding every literal, secret or not, is the only safe rule. The field path
// and positions still say where to look.
//
// It recognises CUE's string and bytes literals, single- or triple-quoted,
// and their #-raw variants. An empty literal is kept: it says what was wrong and hides
// nothing. A single-quoted scanner token name (the parser's "found 'STRING'")
// is kept, as is an apostrophe inside a word. A literal with no closing
// quote hides the rest of the message.
func hideLiterals(msg string) string {
	var b strings.Builder
	for i := 0; i < len(msg); {
		end, body, ok := literalAt(msg, i)
		if !ok {
			b.WriteByte(msg[i])
			i++
			continue
		}
		lit := msg[i:end]
		if body == "" || lit[0] == '\'' && tokenNames()[body] {
			b.WriteString(lit)
		} else {
			b.WriteString(hidden)
		}
		i = end
	}
	return b.String()
}

// literalAt reports whether a literal starts at msg[i] and, if so, where it
// ends and what is between its quotes.
func literalAt(msg string, i int) (end int, body string, ok bool) {
	j := i
	for j < len(msg) && msg[j] == '#' {
		j++
	}
	hashes := msg[i:j]
	if j == len(msg) || msg[j] != '"' && msg[j] != '\'' {
		return 0, "", false
	}
	q := msg[j]
	if q == '\'' && hashes == "" && i > 0 && isWordByte(msg[i-1]) {
		return 0, "", false // an apostrophe: "can't"
	}
	open := string(q)
	if strings.HasPrefix(msg[j:], strings.Repeat(open, 3)) {
		open = strings.Repeat(open, 3)
	}
	start := j + len(open)
	closing := open + hashes
	for k := start; k < len(msg); k++ {
		if hashes == "" && msg[k] == '\\' {
			k++ // skip the escaped byte
			continue
		}
		if strings.HasPrefix(msg[k:], closing) {
			return k + len(closing), msg[start:k], true
		}
	}
	return len(msg), msg[start:], true
}

func isWordByte(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// tokenNames is every CUE token's name ("STRING", "{", "if"), which the
// parser quotes in single quotes in "expected X, found 'T'".
var tokenNames = sync.OnceValue(func() map[string]bool {
	names := map[string]bool{}
	for t := token.Token(0); !strings.HasPrefix(t.String(), "Token("); t++ {
		names[t.String()] = true
	}
	return names
})

// problem is a CUE error with its message replaced and, optionally, one
// more position.
type problem struct {
	inner cueerrors.Error
	msg   string
	extra token.Pos
}

func (p problem) Position() token.Pos  { return p.inner.Position() }
func (p problem) Path() []string       { return p.inner.Path() }
func (p problem) Msg() (string, []any) { return "%s", []any{p.msg} }

func (p problem) InputPositions() []token.Pos {
	if !p.extra.IsValid() {
		return p.inner.InputPositions()
	}
	return append([]token.Pos{p.extra}, p.inner.InputPositions()...)
}

func (p problem) Error() string {
	if path := strings.Join(p.Path(), "."); path != "" {
		return path + ": " + p.msg
	}
	return p.msg
}
