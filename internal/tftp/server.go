// Package tftp реализует TFTP-сервер для раздачи загрузчика iPXE.
package tftp

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pin/tftp/v3"
)

// Config — описывает базовые параметры сервера
type Config struct {
	//Port — UDP-порт для TFTP. По умолчанию 69.
	Port int `yaml:"port"`

	// AssetsPath — путь к папке с файлами для раздачи.
	AssetsPath string `yaml:"assets_path"`
}

// DefaultConfig возвращает Config с дефолтными значениями.
func DefaultConfig() Config {
	return Config{
		Port:       69,
		AssetsPath: "storage/tftp/assets",
	}
}

// State описывает состояние TFTP-сервера.
type State int

// Возможные состояния.
const (
	StateStopped State = iota
	StateRunning
)

// Server — обёртка над pin/tftp с управлением жизненным циклом.
//
// Параметры (адрес, путь к assets) передаются в NewServer через структуру(port, ).
type Server struct {
	addr           string
	assetsPath     string
	realAssetsPath string

	logger *slog.Logger

	srv   *tftp.Server
	conn  net.PacketConn
	state State
	mu    sync.Mutex
	wg    sync.WaitGroup
}

// NewServer создаёт TFTP-сервер.
//
// addr — адрес прослушивания в формате ":69".
// assetsPath — путь к папке с файлами для раздачи.
// logger — обязательный логгер. Паникует, если nil.
func NewServer(cfg Config, logger *slog.Logger) *Server {

	if logger == nil {
		panic("tftp: logger is required")
	}

	absRoot, err := filepath.Abs(cfg.AssetsPath)
	if err != nil {
		panic(fmt.Errorf("tftp: resolve assets path: %w", err))
	}

	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		panic(fmt.Errorf("tftp: resolve real assets path: %w", err))
	}

	s := &Server{
		addr:           fmt.Sprintf(":%d", cfg.Port),
		assetsPath:     cfg.AssetsPath,
		realAssetsPath: realRoot,

		logger: logger.With("component", "tftp"),
	}
	return s
}

func (s State) String() string {
	switch s {
	case StateStopped:
		return "stopped"
	case StateRunning:
		return "running"
	default:
		return "unknown"
	}
}

// Start открывает UDP-сокет и запускает сервер в горутине.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateRunning {
		return errors.New("tftp: already running")
	}

	conn, err := net.ListenPacket("udp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}

	srv := tftp.NewServer(s.handleRead, nil)
	s.conn = conn
	s.srv = srv
	s.state = StateRunning

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {

			s.mu.Lock()
			if s.srv == srv {
				s.state = StateStopped
			}
			s.mu.Unlock()

		}()

		if err := srv.Serve(conn); err != nil {
			s.logger.Error("TFTP server stopped unexpectedly", "err", err)
		}

	}()

	s.logger.Info("TFTP server started", "addr", s.addr, "assets", s.assetsPath)
	return nil
}

// Stop останавливает сервер. Идемпотентен: повторный вызов безвреден.
func (s *Server) Stop() {
	s.mu.Lock()
	if s.state == StateStopped {
		s.mu.Unlock()
		return
	}
	srv := s.srv
	conn := s.conn
	s.state = StateStopped
	s.mu.Unlock()

	srv.Shutdown()
	conn.Close()
	s.wg.Wait()

	s.logger.Info("TFTP server stopped")
}

// Restart перезапускает сервер. Если не запущен — просто запускает.
func (s *Server) Restart() error {
	s.mu.Lock()
	state := s.state
	s.mu.Unlock()

	s.logger.Info("TFTP server restarting...", "state", state)
	s.Stop()
	return s.Start()
}

// Status возвращает текущее состояние сервера
// Возможные значения: StateStopped, StateRunning.
func (s *Server) Status() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// handleRead отдаёт файл из assetsPath.
func (s *Server) handleRead(filename string, rf io.ReaderFrom) error {
	absRoot := s.realAssetsPath
	absFile := filepath.Join(absRoot, filename)

	if !strings.HasPrefix(absFile, absRoot+string(os.PathSeparator)) {
		s.logger.Warn("TFTP rejected suspicious path", "file", filename)
		return errors.New("invalid path")
	}

	realFile, err := filepath.EvalSymlinks(absFile)
	if errors.Is(err, os.ErrNotExist) {
		s.logger.Debug("TFTP file not found", "file", filename)
		return fmt.Errorf("file not found: %s", filename)
	}
	if err != nil {
		s.logger.Error("resolve symlinks", "file", filename, "err", err)
		return fmt.Errorf("resolve path: %w", err)
	}

	// Повторная проверка — уже на развёрнутом пути.
	if !strings.HasPrefix(realFile, absRoot+string(os.PathSeparator)) {
		s.logger.Warn("TFTP rejected symlink escape",
			"file", filename, "real", realFile)
		return errors.New("invalid path via symlink")
	}

	f, err := os.Open(realFile)
	if err != nil {
		s.logger.Error("open file", "file", filename, "err", err)
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		s.logger.Error("stat file", "file", filename, "err", err)
		return fmt.Errorf("stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		s.logger.Warn("TFTP rejected non-regular file",
			"file", filename, "mode", info.Mode())
		return errors.New("not a regular file")
	}

	n, err := rf.ReadFrom(f)
	if err != nil {
		s.logger.Warn("TFTP send failed", "file", filename, "err", err)
		return fmt.Errorf("send file: %w", err)
	}

	s.logger.Debug("TFTP sent", "file", filename, "bytes", n)
	return nil
}
