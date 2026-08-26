package httputil

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetStreamingOutlastsStallTimeoutWhileProgressing(t *testing.T) {
	chunk := make([]byte, 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 10; i++ {
			w.Write(chunk)
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()

	resp, err := getStreaming(context.Background(), srv.URL, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("getStreaming: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != len(chunk)*10 {
		t.Fatalf("got %d bytes, want %d", len(body), len(chunk)*10)
	}
}

func TestGetStreamingFailsOnStalledBody(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("start"))
		w.(http.Flusher).Flush()
		<-release
	}))
	defer srv.Close()
	defer close(release)

	resp, err := getStreaming(context.Background(), srv.URL, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("getStreaming: %v", err)
	}
	defer resp.Body.Close()

	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("expected a stalled body to fail")
	}
}

func TestGetStreamingCancelledByContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("start"))
		w.(http.Flusher).Flush()
		<-release
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := getStreaming(ctx, srv.URL, time.Minute)
	if err != nil {
		cancel()
		t.Fatalf("getStreaming: %v", err)
	}
	defer resp.Body.Close()

	time.AfterFunc(100*time.Millisecond, cancel)
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("expected a cancelled download to fail")
	}
	cancel()
}
