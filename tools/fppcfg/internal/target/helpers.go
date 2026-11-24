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

package target

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// StatusError is a request FPP refused, by HTTP status or by an error status
// in a 200 reply. Message is FPP's own, when it gave one.
type StatusError struct {
	Method  string
	Path    string
	Status  int
	Message string
}

func (e *StatusError) Error() string {
	s := fmt.Sprintf("%s %s: HTTP %d", e.Method, e.Path, e.Status)
	if e.Message != "" {
		s += ": " + e.Message
	}
	return s
}

// GetJSON GETs path and decodes the reply into v.
func GetJSON(ctx context.Context, api API, path string, v any) error {
	resp, err := api.Do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return err
	}
	return decode(http.MethodGet, path, resp, v)
}

// PostJSON POSTs body as JSON to path. A reply that is not 2xx, or whose
// status says "error", is a refusal.
func PostJSON(ctx context.Context, api API, path string, body any) error {
	resp, err := postJSON(ctx, api, path, body)
	if err != nil {
		return err
	}
	return refusal(http.MethodPost, path, resp)
}

// PostJSONOK is PostJSON for the calls that must answer with status "OK";
// see CheckOK.
func PostJSONOK(ctx context.Context, api API, path string, body any) error {
	resp, err := postJSON(ctx, api, path, body)
	if err != nil {
		return err
	}
	return CheckOK(http.MethodPost, path, resp)
}

func postJSON(ctx context.Context, api API, path string, body any) (*Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	return api.Do(ctx, http.MethodPost, path, bytes.NewReader(data), "application/json")
}

// decode checks resp for a refusal and decodes it into v.
func decode(method, path string, resp *Response, v any) error {
	if err := refusal(method, path, resp); err != nil {
		return err
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		return fmt.Errorf("%s %s: bad JSON in reply: %w", method, path, err)
	}
	return nil
}

// refusal returns a StatusError when resp is not 2xx, or is a 200 whose body
// says "Status": "Error" or "status": "ERROR" (FPP uses both).
func refusal(method, path string, resp *Response) error {
	status, message := replyStatus(resp.Body)
	if resp.OK() && !strings.EqualFold(status, "error") {
		return nil
	}
	return &StatusError{Method: method, Path: path, Status: resp.Status, Message: message}
}

// CheckOK returns a StatusError unless resp is a 2xx whose status says OK.
// It is for the calls (network saves and applies, the authorized_keys
// upload) where FPP reports a refusal as a 200 with a status such as
// "ERROR: invalid name" or "Unable to create file", so anything but "OK" is
// one.
func CheckOK(method, path string, resp *Response) error {
	status, message := replyStatus(resp.Body)
	ok := strings.EqualFold(status, "OK")
	if resp.OK() && ok {
		return nil
	}
	if message == "" && !ok {
		message = status
	}
	return &StatusError{Method: method, Path: path, Status: resp.Status, Message: message}
}

// replyStatus reads the status and message FPP puts in its JSON replies,
// under either capitalisation. Some calls print a script's output ahead of
// the JSON (the authorized_keys upload runs installSSHKeys with system()),
// so when the body is not a JSON object as a whole, the first tail of it
// that is one is used. A body with no such object (an array, HTML) has
// neither; one that does not end in "}" is not parsed at all, so an HTML
// error page full of braces costs nothing.
func replyStatus(body []byte) (status, message string) {
	if !bytes.HasSuffix(bytes.TrimSpace(body), []byte("}")) {
		return "", ""
	}
	for i := 0; i < len(body); i++ {
		if i > 0 && body[i] != '{' {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(body[i:], &m) == nil {
			return firstString(m, "Status", "status"), firstString(m, "Message", "message", "error")
		}
	}
	return "", ""
}

// firstString returns the first of keys whose value in m is a non-empty
// JSON string.
func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		var s string
		if json.Unmarshal(m[k], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

// GetSetting reads one setting's definition and value.
func GetSetting(ctx context.Context, api API, name string) (Setting, error) {
	var raw json.RawMessage
	if err := GetJSON(ctx, api, settingPath(name), &raw); err != nil {
		return Setting{}, err
	}
	// FPP 8.4.1 answers a name it neither defines nor has stored with "[]"
	// (PHP's empty array): not defined, no value.
	var m map[string]json.RawMessage
	if string(bytes.TrimSpace(raw)) != "[]" {
		if err := json.Unmarshal(raw, &m); err != nil {
			return Setting{}, fmt.Errorf("GET %s: bad JSON in reply: %w", settingPath(name), err)
		}
	}
	// FPP 10.4 answers an unknown name with only {"value": ...}. The reply's
	// name can differ from the one asked for (initialSetup-02 answers as
	// initialSetup-01), so only its presence counts.
	_, defined := m["name"]
	value := Scalar(m["value"])
	// A checkbox that is neither stored nor given a default has no "value"
	// at all. fppd reads it as 0 (getSettingInt), the value writing false
	// stores, so it reads here as "0".
	if value == "" && Scalar(m["type"]) == "checkbox" {
		value = "0"
	}
	return Setting{
		Defined: defined,
		Value:   value,
		Restart: number(m["restart"]),
		Reboot:  number(m["reboot"]),
		Options: options(m["options"]),
	}, nil
}

// PutSetting writes one setting. FPP's refusal may repeat value, so a caller
// writing a secret scrubs the error (section.scrub).
func PutSetting(ctx context.Context, api API, name, value string) error {
	path := settingPath(name)
	resp, err := api.Do(ctx, http.MethodPut, path, strings.NewReader(value), "text/plain")
	if err != nil {
		return err
	}
	return refusal(http.MethodPut, path, resp)
}

func settingPath(name string) string {
	return "/api/settings/" + url.PathEscape(name)
}

// Scalar returns a JSON string as is and any other value (a number, a
// boolean) as its JSON text; FPP gives values as either ("0" or 0). Absent
// or null is "".
func Scalar(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

// number reads an int that FPP may give as a number or a string; anything
// else is 0.
func number(raw json.RawMessage) int {
	n, _ := strconv.Atoi(Scalar(raw))
	return n
}

// options reads the values of a setting's choices. FPP gives an object of
// label to value (strings or numbers), or for some settings (TimeZone) a
// list whose items are both label and value.
func options(raw json.RawMessage) []string {
	var out []string
	var obj map[string]json.RawMessage
	var list []json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for _, v := range obj {
			out = append(out, Scalar(v))
		}
	} else if json.Unmarshal(raw, &list) == nil {
		for _, v := range list {
			out = append(out, Scalar(v))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// WaitFor calls check every interval until it returns true or ctx ends.
func WaitFor(ctx context.Context, check func(context.Context) bool, every time.Duration) error {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if check(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("gave up waiting: %w", ctx.Err())
		case <-tick.C:
		}
	}
}
