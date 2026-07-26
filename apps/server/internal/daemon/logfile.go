package daemon

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type rotatingLog struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

// OpenRotatingLog keeps one bounded backup at path + ".1". Rotation happens
// before the write that would cross maxBytes, so it does not depend on a
// service restart.
func OpenRotatingLog(path string, maxBytes int64) (io.WriteCloser, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("log size limit must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	w := &rotatingLog{path: path, maxBytes: maxBytes}
	if info, err := os.Stat(path); err == nil && info.Size() >= maxBytes {
		if err := rotateLogPath(path); err != nil {
			return nil, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect log file: %w", err)
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}
	if int64(len(p)) <= w.maxBytes {
		if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
			if err := w.rotate(); err != nil {
				return 0, err
			}
		}
		n, err := w.file.Write(p)
		w.size += int64(n)
		if err != nil {
			return n, err
		}
		if n != len(p) {
			return n, io.ErrShortWrite
		}
		return n, nil
	}
	if w.size > 0 {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	written := 0
	for written < len(p) {
		if w.size >= w.maxBytes {
			if err := w.rotate(); err != nil {
				return written, err
			}
		}
		remaining := w.maxBytes - w.size
		chunkLen := int64(len(p) - written)
		if chunkLen > remaining {
			chunkLen = remaining
		}
		n, err := w.file.Write(p[written : written+int(chunkLen)])
		written += n
		w.size += int64(n)
		if err != nil {
			return written, err
		}
		if int64(n) != chunkLen {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (w *rotatingLog) rotate() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close log before rotation: %w", err)
	}
	w.file = nil
	if err := rotateLogPath(w.path); err != nil {
		if reopenErr := w.open(); reopenErr != nil {
			return fmt.Errorf("%v; reopen active log: %w", err, reopenErr)
		}
		return err
	}
	if err := w.open(); err != nil {
		return err
	}
	return nil
}

func (w *rotatingLog) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotatingLog) open() error {
	file, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("inspect open log file: %w", err)
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func rotateLogPath(path string) error {
	backup := path + ".1"
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove previous log backup: %w", err)
	}
	if err := os.Rename(path, backup); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rotate log file: %w", err)
	}
	return nil
}
