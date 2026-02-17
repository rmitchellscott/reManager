package httputil

import (
	"fmt"
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
