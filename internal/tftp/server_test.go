package tftp

// Тесты для TFTP-сервера.
//
// Запуск:
//   go test ./...
//   go test -race ./...
//
// Все тесты используют случайный свободный порт (127.0.0.1:0),
// временные директории (t.TempDir) и логгер в io.Discard,
// чтобы не засорять вывод.

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/pin/tftp/v3"
)

// ---------- helpers ----------

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestServer создаёт сервер на случайном порту с временной папкой assets.
// Регистрирует Stop на Cleanup. Второй возвращаемый — путь к assets.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		Port:       0,
		AssetsPath: dir,
	}
	srv := NewServer(cfg, newTestLogger())
	t.Cleanup(srv.Stop)
	return srv, dir
}

// startServer запускает сервер и возвращает его реальный адрес host:port.
func startServer(t *testing.T, srv *Server) string {
	t.Helper()
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.conn == nil {
		t.Fatal("conn is nil after Start")
	}
	// LocalAddr может быть [::]:port или 0.0.0.0:port — клиенту нужен
	// конкретный адрес. Берём только порт и подставляем loopback.
	_, port, err := net.SplitHostPort(srv.conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// writeAsset создаёт файл в папке assets (с промежуточными каталогами).
func writeAsset(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// tftpGet скачивает файл и возвращает его содержимое.
// Возвращает error вместо t.Fatal, чтобы тест можно было запускать из горутины.
func tftpGet(addr, filename string) (string, error) {
	client, err := tftp.NewClient(addr)
	if err != nil {
		return "", err
	}
	r, err := client.Receive(filename, "octet")
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if _, err := r.WriteTo(&buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ---------- State.String() ----------

func TestStateString(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{StateStopped, "stopped"},
		{StateRunning, "running"},
		{State(42), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

// ---------- lifecycle ----------

func TestStatusTransitions(t *testing.T) {
	srv, _ := newTestServer(t)

	if got := srv.Status(); got != StateStopped {
		t.Errorf("initial: want %v, got %v", StateStopped, got)
	}

	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if got := srv.Status(); got != StateRunning {
		t.Errorf("after start: want %v, got %v", StateRunning, got)
	}

	srv.Stop()
	if got := srv.Status(); got != StateStopped {
		t.Errorf("after stop: want %v, got %v", StateStopped, got)
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	srv, _ := newTestServer(t)

	if err := srv.Start(); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if err := srv.Start(); err == nil {
		t.Error("second start: want error, got nil")
	}
	if got := srv.Status(); got != StateRunning {
		t.Errorf("state after failed start: want running, got %v", got)
	}
}

func TestStopIdempotent(t *testing.T) {
	srv, _ := newTestServer(t)
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Stop()
		srv.Stop()
		srv.Stop()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung")
	}

	if got := srv.Status(); got != StateStopped {
		t.Errorf("want stopped, got %v", got)
	}
}

func TestStopBeforeStart(t *testing.T) {
	srv, _ := newTestServer(t)

	// Не должен падать и не должен висеть.
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Stop()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung before Start")
	}

	if got := srv.Status(); got != StateStopped {
		t.Errorf("want stopped, got %v", got)
	}
}

func TestRestart(t *testing.T) {
	srv, _ := newTestServer(t)

	// Restart на остановленном — запускает.
	if err := srv.Restart(); err != nil {
		t.Fatalf("restart stopped: %v", err)
	}
	if got := srv.Status(); got != StateRunning {
		t.Errorf("after restart on stopped: want running, got %v", got)
	}

	// Restart на запущенном — перезапускает.
	if err := srv.Restart(); err != nil {
		t.Fatalf("restart running: %v", err)
	}
	if got := srv.Status(); got != StateRunning {
		t.Errorf("after restart on running: want running, got %v", got)
	}

	// После явного Stop состояние Stopped.
	srv.Stop()
	if got := srv.Status(); got != StateStopped {
		t.Errorf("after stop: want stopped, got %v", got)
	}
}

func TestConcurrentStartStop(t *testing.T) {
	srv, _ := newTestServer(t)

	const goroutines = 20
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = srv.Start()
			_ = srv.Status()
			srv.Stop()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("concurrent Start/Stop hung")
	}
}

// ---------- handleRead: happy path ----------

func TestReadFile(t *testing.T) {
	srv, dir := newTestServer(t)
	want := "fake ipxe content"
	writeAsset(t, dir, "ipxe.efi", want)
	addr := startServer(t, srv)

	got, err := tftpGet(addr, "ipxe.efi")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != want {
		t.Errorf("content: want %q, got %q", want, got)
	}
}

func TestReadFileInSubdir(t *testing.T) {
	srv, dir := newTestServer(t)
	writeAsset(t, dir, "sub/ipxe.efi", "hello")
	addr := startServer(t, srv)

	got, err := tftpGet(addr, "sub/ipxe.efi")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "hello" {
		t.Errorf("content: want %q, got %q", "hello", got)
	}
}

// ---------- handleRead: отказы ----------

func TestPathTraversal(t *testing.T) {
	srv, dir := newTestServer(t)
	writeAsset(t, dir, "safe.txt", "ok")
	addr := startServer(t, srv)

	cases := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"safe/../../etc/passwd",
		"..",
		"../",
	}
	for _, name := range cases {
		name := name
		t.Run(name, func(t *testing.T) {
			if _, err := tftpGet(addr, name); err == nil {
				t.Errorf("want error for %q, got nil", name)
			}
		})
	}
}

func TestFileNotFound(t *testing.T) {
	srv, dir := newTestServer(t)
	writeAsset(t, dir, "safe.txt", "ok")
	addr := startServer(t, srv)

	if _, err := tftpGet(addr, "missing.efi"); err == nil {
		t.Error("want error for missing file, got nil")
	}
}

func TestDirectoryRejected(t *testing.T) {
	srv, dir := newTestServer(t)
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	addr := startServer(t, srv)

	if _, err := tftpGet(addr, "subdir"); err == nil {
		t.Error("want error for directory, got nil")
	}
}

// ---------- симлинки ----------

func TestSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}

	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	assetsDir := filepath.Join(base, "assets")
	if err := os.Mkdir(assetsDir, 0755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.Symlink(
		filepath.Join(outside, "secret.txt"),
		filepath.Join(assetsDir, "evil"),
	); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	srv := NewServer(Config{
		Port:       0,
		AssetsPath: assetsDir,
	}, newTestLogger())
	t.Cleanup(srv.Stop)
	addr := startServer(t, srv)

	if _, err := tftpGet(addr, "evil"); err == nil {
		t.Error("want error for symlink escape, got nil")
	}
}

func TestSymlinkInside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}

	srv, dir := newTestServer(t)
	writeAsset(t, dir, "real.efi", "real content")
	if err := os.Symlink("real.efi", filepath.Join(dir, "alias.efi")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	addr := startServer(t, srv)

	got, err := tftpGet(addr, "alias.efi")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "real content" {
		t.Errorf("content: want %q, got %q", "real content", got)
	}
}

func TestAssetsIsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}

	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "ipxe.efi"), []byte("content"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	linkDir := filepath.Join(base, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	srv := NewServer(Config{
		Port:       0,
		AssetsPath: linkDir,
	}, newTestLogger())

	t.Cleanup(srv.Stop)
	addr := startServer(t, srv)

	got, err := tftpGet(addr, "ipxe.efi")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != "content" {
		t.Errorf("content: want %q, got %q", "content", got)
	}
}
