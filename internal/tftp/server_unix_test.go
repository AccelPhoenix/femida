//go:build unix

package tftp

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestFIFORejected проверяет, что сервер не пытается открыть FIFO —
// иначе os.Open заблокируется, пока кто-то не откроет другой конец.
func TestFIFORejected(t *testing.T) {
	srv, dir := newTestServer(t)
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo"), 0644); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	addr := startServer(t, srv)

	done := make(chan struct{})
	var getErr error
	go func() {
		defer close(done)
		_, getErr = tftpGet(addr, "fifo")
	}()

	select {
	case <-done:
		if getErr == nil {
			t.Error("want error for FIFO, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request hung on FIFO — os.Open blocked")
	}
}
