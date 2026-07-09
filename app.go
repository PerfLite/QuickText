package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"

	"github.com/creack/pty"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type FileNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	IsDir    bool        `json:"isDir"`
	Children []*FileNode `json:"children,omitempty"`
}

type Terminal struct {
	cmd    *exec.Cmd
	ptmx   *os.File
	cancel context.CancelFunc
}

type App struct {
	ctx        context.Context
	canClose   bool
	mu         sync.Mutex
	terminal   *Terminal
	termMu     sync.Mutex
	pendingFile string
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetPendingFile() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.pendingFile
	a.pendingFile = ""
	return f
}

func (a *App) BeforeClose(ctx context.Context) bool {
	if a.canClose {
		return false
	}
	runtime.EventsEmit(ctx, "user_wants_to_close")
	return true
}

func (a *App) ForceClose() {
	a.mu.Lock()
	a.canClose = true
	a.mu.Unlock()
	runtime.Quit(a.ctx)
}

func (a *App) OpenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) SaveFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func (a *App) OpenFileChooser() (string, error) {
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Открыть файл",
		Filters: []runtime.FileFilter{
			{DisplayName: "Все файлы (*)", Pattern: "*"},
			{DisplayName: "Текстовые (*.txt *.md *.log)", Pattern: "*.txt;*.md;*.log"},
			{DisplayName: "Исходники (*.go *.js *.ts *.py *.rs *.c *.cpp *.h)", Pattern: "*.go;*.js;*.ts;*.py;*.rs;*.c;*.cpp;*.h"},
			{DisplayName: "Веб (*.html *.css *.vue *.json *.yaml)", Pattern: "*.html;*.css;*.vue;*.json;*.yaml;*.yml;*.toml"},
		},
	})
	return file, err
}

func (a *App) SaveFileChooser() (string, error) {
	file, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Сохранить файл как",
		Filters: []runtime.FileFilter{
			{DisplayName: "Все файлы (*)", Pattern: "*"},
			{DisplayName: "Текстовые (*.txt *.md)", Pattern: "*.txt;*.md"},
		},
	})
	return file, err
}

func (a *App) GetFolderTree(rootPath string) ([]*FileNode, error) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	var nodes []*FileNode
	for _, entry := range entries {
		if entry.Name()[0] == '.' {
			continue
		}
		node := &FileNode{
			Name:  entry.Name(),
			Path:  filepath.Join(rootPath, entry.Name()),
			IsDir: entry.IsDir(),
		}
		if entry.IsDir() {
			children, err := a.GetFolderTree(node.Path)
			if err == nil {
				node.Children = children
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// --- Terminal ---

func (a *App) StartTerminal() error {
	a.termMu.Lock()
	defer a.termMu.Unlock()

	if a.terminal != nil {
		return nil
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	ctx, cancel := context.WithCancel(a.ctx)
	cmd := exec.CommandContext(ctx, shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		cancel()
		return err
	}

	a.terminal = &Terminal{
		cmd:    cmd,
		ptmx:   ptmx,
		cancel: cancel,
	}

	// Read PTY output and emit to frontend
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				runtime.EventsEmit(a.ctx, "terminal-output", string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
		runtime.EventsEmit(a.ctx, "terminal-closed")
	}()

	return nil
}

func (a *App) WriteTerminal(data string) error {
	a.termMu.Lock()
	defer a.termMu.Unlock()

	if a.terminal == nil {
		return nil
	}
	_, err := a.terminal.ptmx.Write([]byte(data))
	return err
}

func (a *App) ResizeTerminal(cols, rows uint) error {
	a.termMu.Lock()
	defer a.termMu.Unlock()

	if a.terminal == nil {
		return nil
	}
	return pty.Setsize(a.terminal.ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

func (a *App) CloseTerminal() {
	a.termMu.Lock()
	defer a.termMu.Unlock()

	if a.terminal == nil {
		return
	}
	a.terminal.cancel()
	a.terminal.ptmx.Close()
	a.terminal.cmd.Process.Kill()
	a.terminal = nil
}

// --- Autosave ---

func (a *App) getAutoSaveDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	dir := home + "/.local/share/QuickText/autosave"
	os.MkdirAll(dir, 0755)
	return dir
}

func (a *App) SaveAutoSave(data string) error {
	return os.WriteFile(a.getAutoSaveDir()+"/autosave.json", []byte(data), 0644)
}

func (a *App) LoadAutoSave() string {
	data, err := os.ReadFile(a.getAutoSaveDir() + "/autosave.json")
	if err != nil {
		return ""
	}
	return string(data)
}

func (a *App) ClearAutoSave() {
	os.Remove(a.getAutoSaveDir() + "/autosave.json")
}
