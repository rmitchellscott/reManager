package tablettest

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type httpExchange struct {
	Method      string `json:"method"`
	URL         string `json:"url"`
	Status      int    `json:"status"`
	ContentType string `json:"contentType,omitempty"`
	Body        []byte `json:"body"`
}

// Loopback requests pass through, since they go to servers the test runs itself.
type HTTPCassette struct {
	dir      string
	record   bool
	upstream http.RoundTripper
	onMiss   func(url string)
}

func NewHTTPCassette(dir string, record bool, upstream http.RoundTripper, onMiss func(url string)) *HTTPCassette {
	return &HTTPCassette{dir: dir, record: record, upstream: upstream, onMiss: onMiss}
}

func (c *HTTPCassette) RoundTrip(req *http.Request) (*http.Response, error) {
	if ip := net.ParseIP(req.URL.Hostname()); (ip != nil && ip.IsLoopback()) || req.URL.Hostname() == "localhost" {
		return c.upstream.RoundTrip(req)
	}
	key := req.Method + " " + req.URL.String()
	sum := sha256.Sum256([]byte(key))
	path := filepath.Join(c.dir, hex.EncodeToString(sum[:8])+".json.gz")

	if c.record {
		resp, err := c.upstream.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		exchange := httpExchange{req.Method, req.URL.String(), resp.StatusCode, resp.Header.Get("Content-Type"), body}
		if err := writeGzipJSON(path, exchange); err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, nil
	}

	var exchange httpExchange
	if err := readGzipJSON(path, &exchange); err != nil {
		c.onMiss(key)
		return nil, fmt.Errorf("tablettest: no recorded response for %s", key)
	}
	header := http.Header{}
	if exchange.ContentType != "" {
		header.Set("Content-Type", exchange.ContentType)
	}
	return &http.Response{
		Status:        http.StatusText(exchange.Status),
		StatusCode:    exchange.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(exchange.Body)),
		ContentLength: int64(len(exchange.Body)),
		Request:       req,
	}, nil
}

func writeGzipJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(value); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func readGzipJSON(path string, into any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	return json.NewDecoder(zr).Decode(into)
}
