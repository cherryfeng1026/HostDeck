package collector_test

import (
	"testing"

	"hostdeck/server/internal/collector"
)

func TestParseMemInfo(t *testing.T) {
	raw := "MemTotal: 2048000 kB\nMemAvailable: 1024000 kB\n"
	usage, err := collector.ParseMemInfo(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if usage != 50 {
		t.Fatalf("expected 50, got %v", usage)
	}
}

func TestParseDF(t *testing.T) {
	raw := "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 100000 55000 45000 55% /\n"
	usage, err := collector.ParseDF(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if usage != 55 {
		t.Fatalf("expected 55, got %v", usage)
	}
}

func TestParseLoadAvg(t *testing.T) {
	raw := "0.15 0.20 0.25 1/100 12345\n"
	load1, load5, load15, err := collector.ParseLoadAvg(raw)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if load1 != 0.15 || load5 != 0.20 || load15 != 0.25 {
		t.Fatalf("unexpected loads: %v %v %v", load1, load5, load15)
	}
}
