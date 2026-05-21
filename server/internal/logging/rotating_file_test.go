package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingFileWriterRotatesCompressesAndLimitsBackups(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewRotatingFileWriter(RotatingFileConfig{
		Filename:   filepath.Join(dir, "hostdeck.log"),
		MaxSizeMB:  1,
		MaxBackups: 1,
		MaxAgeDays: 14,
		Compress:   true,
	})
	if err != nil {
		t.Fatalf("new rotating writer: %v", err)
	}
	defer func() {
		if err := writer.Close(); err != nil {
			t.Fatalf("close writer: %v", err)
		}
	}()

	chunk := append(bytes.Repeat([]byte("a"), 700*1024), '\n')
	for i := 0; i < 3; i++ {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatalf("write chunk %d: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	rotated := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "hostdeck-") && strings.HasSuffix(entry.Name(), ".log.gz") {
			rotated++
		}
	}
	if rotated != 1 {
		t.Fatalf("expected exactly 1 compressed rotated backup, got %d entries=%v", rotated, entries)
	}
	if _, err := os.Stat(filepath.Join(dir, "hostdeck.log")); err != nil {
		t.Fatalf("expected current log file: %v", err)
	}
}
