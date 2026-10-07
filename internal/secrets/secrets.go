// Package secrets handles the plaintext secrets payload: an ordered set of
// KEY=VALUE pairs in dotenv style. The payload is only ever held in memory;
// it is encrypted before being written to .envis.
package secrets

import (
	"bufio"
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// Set is an ordered collection of environment variables.
type Set struct {
	keys   []string
	values map[string]string
}

// New returns an empty Set.
func New() *Set {
	return &Set{values: make(map[string]string)}
}

// Parse reads a dotenv-style payload into a Set. Blank lines and lines
// starting with '#' are ignored. A leading "export " is stripped.
func Parse(data []byte) (*Set, error) {
	s := New()
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return nil, fmt.Errorf("line %d: missing '=' in %q", lineNo, line)
		}
		key := strings.TrimSpace(line[:eq])
		val := line[eq+1:]
		val = unquote(strings.TrimSpace(val))
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNo)
		}
		s.Set(key, val)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return s, nil
}

// unquote removes matching surrounding single or double quotes.
func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// Set inserts or updates a key, preserving insertion order for new keys.
func (s *Set) Set(key, value string) {
	if _, ok := s.values[key]; !ok {
		s.keys = append(s.keys, key)
	}
	s.values[key] = value
}

// Get returns the value for a key and whether it exists.
func (s *Set) Get(key string) (string, bool) {
	v, ok := s.values[key]
	return v, ok
}

// Delete removes a key. Returns true if the key existed.
func (s *Set) Delete(key string) bool {
	if _, ok := s.values[key]; !ok {
		return false
	}
	delete(s.values, key)
	for i, k := range s.keys {
		if k == key {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			break
		}
	}
	return true
}

// Keys returns the keys in sorted order.
func (s *Set) Keys() []string {
	out := make([]string, len(s.keys))
	copy(out, s.keys)
	sort.Strings(out)
	return out
}

// Len returns the number of secrets.
func (s *Set) Len() int { return len(s.keys) }

// Serialize renders the Set as a dotenv-style payload, sorted by key for
// deterministic output.
func (s *Set) Serialize() []byte {
	var b bytes.Buffer
	for _, k := range s.Keys() {
		fmt.Fprintf(&b, "%s=%s\n", k, s.values[k])
	}
	return b.Bytes()
}

// EnvLines returns the secrets as KEY=VALUE strings suitable for os.Environ,
// sorted by key.
func (s *Set) EnvLines() []string {
	keys := s.Keys()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+s.values[k])
	}
	return out
}
