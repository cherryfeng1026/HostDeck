package service

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestResolveAlertWebhookHostRejectsPrivateDNSResult(t *testing.T) {
	previousLookup := alertWebhookLookupIPAddr
	alertWebhookLookupIPAddr = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.5")}}, nil
	}
	t.Cleanup(func() {
		alertWebhookLookupIPAddr = previousLookup
	})

	_, err := resolveAlertWebhookHost(context.Background(), "hooks.example.test")
	if err == nil || !strings.Contains(err.Error(), "must not resolve") {
		t.Fatalf("expected private DNS result rejection, got %v", err)
	}
}

func TestValidateAlertWebhookURLRejectsCredentials(t *testing.T) {
	if _, err := NewWebhookAlertNotifier("https://user:pass@hooks.example.test/alerts", 0); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("expected credential validation error, got %v", err)
	}
}
