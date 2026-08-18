package provider

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// errNotFound lets resource Reads distinguish "deleted out of band" (drop
// from state) from real API failures.
var errNotFound = errors.New("resource not found")

type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newAPIClient(baseURL, token string, insecureSkipVerify bool) *apiClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &apiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

// do runs a request and decodes the JSON object response with UseNumber, so
// numeric fields survive regardless of whether the API renders them as JSON
// numbers or strings (MySQL decimal columns arrive as strings).
func (c *apiClient) do(method, path string, payload map[string]any) (map[string]any, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s returned HTTP %d: %s", method, path, resp.StatusCode, truncate(string(raw), 600))
	}
	if resp.StatusCode == http.StatusNoContent || len(raw) == 0 {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	decoded := map[string]any{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%s %s returned unparseable JSON: %w", method, path, err)
	}

	return decoded, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}

	return s[:max] + "..."
}

/* ------------------------------------------------------------------
| Response field coercion
|
| The API serializes Eloquent attributes directly, so booleans can be
| 0/1, decimals can be strings, and ids can be numbers. These helpers
| absorb that variance in one place.
|------------------------------------------------------------------ */

func fieldString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	switch value := v.(type) {
	case string:
		return value, true
	case json.Number:
		return value.String(), true
	}

	return "", false
}

func fieldFloat(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch value := v.(type) {
	case json.Number:
		f, err := value.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(value, 64)
		return f, err == nil
	}

	return 0, false
}

func fieldInt(m map[string]any, key string) (int64, bool) {
	f, ok := fieldFloat(m, key)
	if !ok {
		return 0, false
	}

	return int64(f), true
}

func fieldBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return false, false
	}
	switch value := v.(type) {
	case bool:
		return value, true
	case json.Number:
		return value.String() != "0", true
	case string:
		return value == "1" || value == "true", true
	}

	return false, false
}

func fieldStringSlice(m map[string]any, key string) ([]string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return nil, false
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, false
	}

	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, isString := item.(string); isString {
			out = append(out, s)
		}
	}

	return out, true
}

func fieldStringMap(m map[string]any, key string) (map[string]string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return nil, false
	}
	raw, ok := v.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil, false
	}

	out := make(map[string]string, len(raw))
	for k, item := range raw {
		if s, isString := item.(string); isString {
			out[k] = s
		}
	}

	return out, true
}
