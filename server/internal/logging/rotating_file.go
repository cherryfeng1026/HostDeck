package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RotatingFileConfig struct {
	Filename   string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

type RotatingFileWriter struct {
	mu         sync.Mutex
	filename   string
	maxSize    int64
	maxBackups int
	maxAge     time.Duration
	compress   bool
	file       *os.File
	size       int64
}

func NewRotatingFileWriter(cfg RotatingFileConfig) (*RotatingFileWriter, error) {
	filename := strings.TrimSpace(cfg.Filename)
	if filename == "" {
		return nil, fmt.Errorf("日志文件路径不能为空")
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 50
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 14
	}
	if cfg.MaxAgeDays <= 0 {
		cfg.MaxAgeDays = 14
	}
	writer := &RotatingFileWriter{
		filename:   filename,
		maxSize:    int64(cfg.MaxSizeMB) * 1024 * 1024,
		maxBackups: cfg.MaxBackups,
		maxAge:     time.Duration(cfg.MaxAgeDays) * 24 * time.Hour,
		compress:   cfg.Compress,
	}
	if err := writer.open(); err != nil {
		return nil, err
	}
	if err := writer.cleanup(time.Now()); err != nil {
		_ = writer.Close()
		return nil, err
	}
	return writer, nil
}

func (w *RotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(time.Now()); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closeLocked()
}

func (w *RotatingFileWriter) open() error {
	if err := os.MkdirAll(filepath.Dir(w.filename), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(w.filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *RotatingFileWriter) closeLocked() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.size = 0
	return err
}

func (w *RotatingFileWriter) rotate(now time.Time) error {
	if err := w.closeLocked(); err != nil {
		return err
	}
	if info, err := os.Stat(w.filename); err == nil && info.Size() > 0 {
		rotated := w.rotatedFilename(now)
		if err := os.Rename(w.filename, rotated); err != nil {
			return err
		}
		if w.compress {
			if err := compressFile(rotated); err != nil {
				return err
			}
		}
	}
	if err := w.open(); err != nil {
		return err
	}
	return w.cleanup(now)
}

func (w *RotatingFileWriter) rotatedFilename(now time.Time) string {
	dir := filepath.Dir(w.filename)
	ext := filepath.Ext(w.filename)
	base := strings.TrimSuffix(filepath.Base(w.filename), ext)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", base, now.Local().Format("20060102T150405.000000000"), ext))
}

func (w *RotatingFileWriter) cleanup(now time.Time) error {
	files, err := w.rotatedFiles()
	if err != nil {
		return err
	}
	cutoff := now.Add(-w.maxAge)
	kept := make([]rotatedFile, 0, len(files))
	for _, item := range files {
		if item.modTime.Before(cutoff) {
			if err := os.Remove(item.path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		kept = append(kept, item)
	}
	sort.Slice(kept, func(i, j int) bool {
		return kept[i].modTime.After(kept[j].modTime)
	})
	for index, item := range kept {
		if index < w.maxBackups {
			continue
		}
		if err := os.Remove(item.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

type rotatedFile struct {
	path    string
	modTime time.Time
}

func (w *RotatingFileWriter) rotatedFiles() ([]rotatedFile, error) {
	dir := filepath.Dir(w.filename)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	ext := filepath.Ext(w.filename)
	base := strings.TrimSuffix(filepath.Base(w.filename), ext)
	prefix := base + "-"
	files := make([]rotatedFile, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !strings.HasSuffix(name, ext) && !strings.HasSuffix(name, ext+".gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, rotatedFile{
			path:    filepath.Join(dir, name),
			modTime: info.ModTime(),
		})
	}
	return files, nil
}

func compressFile(path string) error {
	gzipPath := path + ".gz"
	source, err := os.Open(path)
	if err != nil {
		return err
	}

	tempPath := gzipPath + ".tmp"
	target, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		_ = source.Close()
		return err
	}
	gzipWriter := gzip.NewWriter(target)
	_, copyErr := io.Copy(gzipWriter, source)
	sourceCloseErr := source.Close()
	closeErr := gzipWriter.Close()
	fileCloseErr := target.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	if sourceCloseErr != nil {
		_ = os.Remove(tempPath)
		return sourceCloseErr
	}
	if fileCloseErr != nil {
		_ = os.Remove(tempPath)
		return fileCloseErr
	}
	if err := os.Rename(tempPath, gzipPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return os.Remove(path)
}
