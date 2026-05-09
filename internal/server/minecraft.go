package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/kohanmathers/kmresolv/internal/config"
	"github.com/kohanmathers/kmresolv/internal/logger"
)

type MinecraftServer struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cfg    *config.MinecraftConfig
	apiURL string
}

func NewMinecraftServer(cfg *config.MinecraftConfig, apiURL string) *MinecraftServer {
	return &MinecraftServer{cfg: cfg, apiURL: apiURL}
}

func (m *MinecraftServer) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil && m.cmd.ProcessState == nil {
		return fmt.Errorf("minecraft server already running")
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find executable path: %w", err)
	}
	jarPath := filepath.Join(filepath.Dir(exe), "kmresolv-1.0.0-SNAPSHOT.jar")

	if _, err := os.Stat(jarPath); err != nil {
		return fmt.Errorf("minecraft jar not found at %s", jarPath)
	}

	args := []string{
		fmt.Sprintf("-Xms%s", m.cfg.MinRAM),
		fmt.Sprintf("-Xmx%s", m.cfg.MaxRAM),
		"-jar", jarPath,
		"--addr", m.cfg.Listen,
		"--port", fmt.Sprintf("%d", m.cfg.Port),
		"--api", m.apiURL,
	}

	m.cmd = exec.Command("java", args...)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start minecraft server: %w", err)
	}

	logger.LogInfo("minecraft server started (pid %d) on %s:%d", m.cmd.Process.Pid, m.cfg.Listen, m.cfg.Port)

	go func() {
		err := m.cmd.Wait()
		if err != nil {
			logger.LogWarn("minecraft server exited: %v", err)
		} else {
			logger.LogInfo("minecraft server stopped cleanly")
		}
		m.mu.Lock()
		m.cmd = nil
		m.mu.Unlock()
	}()

	return nil
}

func (m *MinecraftServer) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	if err := m.cmd.Process.Signal(os.Interrupt); err != nil {
		return m.cmd.Process.Kill()
	}
	return nil
}

func (m *MinecraftServer) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cmd != nil && m.cmd.ProcessState == nil
}
