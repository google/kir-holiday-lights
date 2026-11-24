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
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// scrub returns err with every secret masked out of its text: as written,
// as %q shows it, and as JSON escapes it, since FPP or a transport may
// repeat a request body either way. An error that carries no secret is
// returned unchanged.
func scrub(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := scrubText(err.Error(), secrets...)
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

func scrubText(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		q := strconv.Quote(s)
		j, _ := json.Marshal(s)
		for _, form := range []string{s, q[1 : len(q)-1], string(j[1 : len(j)-1])} {
			msg = strings.ReplaceAll(msg, form, "[secret]")
		}
	}
	return msg
}
