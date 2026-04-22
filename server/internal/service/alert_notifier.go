package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hostdeck/server/internal/domain"
)

type AlertNotification struct {
	EventType  string            `json:"eventType"`
	Alert      domain.AlertEvent `json:"alert"`
	OccurredAt time.Time         `json:"occurredAt"`
}

type AlertNotifier interface {
	NotifyAlert(ctx context.Context, notification AlertNotification) error
}

type AlertNotificationSettingsReader interface {
	GetNotificationSettings(ctx context.Context) (domain.AlertNotificationSettings, error)
}

type WebhookAlertNotifier struct {
	client     *http.Client
	webhookURL string
}

type DynamicWebhookAlertNotifier struct {
	settings AlertNotificationSettingsReader
}

func NewWebhookAlertNotifier(webhookURL string, timeout time.Duration) (*WebhookAlertNotifier, error) {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return nil, fmt.Errorf("alert webhook url is required")
	}
	parsedURL, err := url.ParseRequestURI(webhookURL)
	if err != nil {
		return nil, fmt.Errorf("parse alert webhook url: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("alert webhook url must use http or https")
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("alert webhook url host is required")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookAlertNotifier{
		client:     &http.Client{Timeout: timeout},
		webhookURL: webhookURL,
	}, nil
}

func NewDynamicWebhookAlertNotifier(settings AlertNotificationSettingsReader) *DynamicWebhookAlertNotifier {
	return &DynamicWebhookAlertNotifier{settings: settings}
}

func (n *WebhookAlertNotifier) NotifyAlert(ctx context.Context, notification AlertNotification) error {
	if n == nil || n.webhookURL == "" {
		return nil
	}
	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("marshal alert notification: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build alert webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := n.client.Do(request)
	if err != nil {
		return fmt.Errorf("send alert webhook request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	message := strings.TrimSpace(string(responseBody))
	if message == "" {
		return fmt.Errorf("alert webhook returned status %d", response.StatusCode)
	}
	return fmt.Errorf("alert webhook returned status %d: %s", response.StatusCode, message)
}

func (n *DynamicWebhookAlertNotifier) NotifyAlert(ctx context.Context, notification AlertNotification) error {
	if n == nil || n.settings == nil {
		return nil
	}
	settings, err := n.settings.GetNotificationSettings(ctx)
	if err != nil {
		return fmt.Errorf("load alert notification settings: %w", err)
	}
	if !settings.Enabled {
		return nil
	}
	webhookNotifier, err := NewWebhookAlertNotifier(settings.WebhookURL, time.Duration(settings.WebhookTimeoutSeconds)*time.Second)
	if err != nil {
		return err
	}
	return webhookNotifier.NotifyAlert(ctx, notification)
}
