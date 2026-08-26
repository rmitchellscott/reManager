package httputil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"time"
)

var userAgent string

func Init(version string) {
	userAgent = fmt.Sprintf("reManager/%s (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
}

type uaTransport struct {
	base http.RoundTripper
}

func (t *uaTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", userAgent)
	return t.base.RoundTrip(req)
}

func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &uaTransport{base: http.DefaultTransport},
	}
}

const stallTimeout = 60 * time.Second

var streamingClient = &http.Client{Transport: newStreamingTransport()}

func newStreamingTransport() http.RoundTripper {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.ResponseHeaderTimeout = 30 * time.Second
	return &uaTransport{base: base}
}

func GetStreaming(ctx context.Context, url string) (*http.Response, error) {
	return getStreaming(ctx, url, stallTimeout)
}

func getStreaming(ctx context.Context, url string, timeout time.Duration) (*http.Response, error) {
	ctx, cancel := context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, err
	}

	resp, err := streamingClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}

	resp.Body = &stallGuardedBody{
		body:    resp.Body,
		cancel:  cancel,
		timeout: timeout,
		timer:   time.AfterFunc(timeout, cancel),
	}
	return resp, nil
}

type stallGuardedBody struct {
	body    io.ReadCloser
	cancel  context.CancelFunc
	timeout time.Duration
	timer   *time.Timer
}

func (b *stallGuardedBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		b.timer.Reset(b.timeout)
	}
	return n, err
}

func (b *stallGuardedBody) Close() error {
	b.timer.Stop()
	err := b.body.Close()
	b.cancel()
	return err
}
