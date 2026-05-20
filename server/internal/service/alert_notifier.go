package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
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

type TransactionalAlertNotifier interface {
	PrepareDelivery(ctx context.Context, notification AlertNotification) (domain.AlertNotificationDelivery, string, bool, error)
	DeliverPrepared(ctx context.Context, delivery domain.AlertNotificationDelivery, notification AlertNotification) error
}

type AlertNotificationSettingsReader interface {
	GetNotificationSettings(ctx context.Context) (domain.AlertNotificationSettings, error)
}

type AlertNotificationDeliveryStore interface {
	CreateNotificationDelivery(ctx context.Context, delivery domain.AlertNotificationDelivery, payload string) (domain.AlertNotificationDelivery, error)
	RecordNotificationDeliveryAttempt(ctx context.Context, deliveryID int64, status string, lastError string, nextAttemptAt *time.Time, attemptedAt time.Time) error
	ListNotificationDeliveries(ctx context.Context, filter domain.AlertNotificationDeliveryFilter) ([]domain.AlertNotificationDelivery, error)
}

type WebhookAlertNotifier struct {
	client     *http.Client
	webhookURL string
}

type DynamicWebhookAlertNotifier struct {
	settings   AlertNotificationSettingsReader
	deliveries AlertNotificationDeliveryStore
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
	if err := validateAlertWebhookURL(parsedURL); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookAlertNotifier{
		client:     newAlertWebhookHTTPClient(timeout),
		webhookURL: webhookURL,
	}, nil
}

func NewDynamicWebhookAlertNotifier(settings AlertNotificationSettingsReader, deliveries ...AlertNotificationDeliveryStore) *DynamicWebhookAlertNotifier {
	notifier := &DynamicWebhookAlertNotifier{settings: settings}
	if len(deliveries) > 0 {
		notifier.deliveries = deliveries[0]
	}
	return notifier
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
	if n.deliveries == nil {
		return webhookNotifier.NotifyAlert(ctx, notification)
	}
	delivery, err := n.createDelivery(ctx, notification)
	if err != nil {
		return err
	}
	return n.deliver(ctx, webhookNotifier, delivery, notification)
}

func (n *DynamicWebhookAlertNotifier) PrepareDelivery(ctx context.Context, notification AlertNotification) (domain.AlertNotificationDelivery, string, bool, error) {
	if n == nil || n.settings == nil || n.deliveries == nil {
		return domain.AlertNotificationDelivery{}, "", false, nil
	}
	settings, err := n.settings.GetNotificationSettings(ctx)
	if err != nil {
		return domain.AlertNotificationDelivery{}, "", false, fmt.Errorf("load alert notification settings: %w", err)
	}
	if !settings.Enabled {
		return domain.AlertNotificationDelivery{}, "", false, nil
	}
	if _, err := NewWebhookAlertNotifier(settings.WebhookURL, time.Duration(settings.WebhookTimeoutSeconds)*time.Second); err != nil {
		return domain.AlertNotificationDelivery{}, "", false, err
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		return domain.AlertNotificationDelivery{}, "", false, fmt.Errorf("marshal alert notification delivery: %w", err)
	}
	occurredAt := notification.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return domain.AlertNotificationDelivery{
		EventType:  notification.EventType,
		AlertID:    notification.Alert.ID,
		RuleID:     notification.Alert.RuleID,
		ServerID:   notification.Alert.ServerID,
		ServerName: notification.Alert.ServerName,
		Status:     domain.AlertNotificationDeliveryPending,
		OccurredAt: occurredAt,
	}, string(payload), true, nil
}

func (n *DynamicWebhookAlertNotifier) DeliverPrepared(ctx context.Context, delivery domain.AlertNotificationDelivery, notification AlertNotification) error {
	if n == nil || n.settings == nil || n.deliveries == nil {
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
	return n.deliver(ctx, webhookNotifier, delivery, notification)
}

func (n *DynamicWebhookAlertNotifier) RetryPending(ctx context.Context, limit int) (int, error) {
	if n == nil || n.settings == nil || n.deliveries == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	settings, err := n.settings.GetNotificationSettings(ctx)
	if err != nil {
		return 0, fmt.Errorf("load alert notification settings: %w", err)
	}
	if !settings.Enabled {
		return 0, nil
	}
	webhookNotifier, err := NewWebhookAlertNotifier(settings.WebhookURL, time.Duration(settings.WebhookTimeoutSeconds)*time.Second)
	if err != nil {
		return 0, err
	}
	deliveries, err := n.deliveries.ListNotificationDeliveries(ctx, domain.AlertNotificationDeliveryFilter{
		Limit:   limit,
		DueOnly: true,
		Now:     time.Now().UTC(),
	})
	if err != nil {
		return 0, err
	}
	retried := 0
	var lastErr error
	for _, delivery := range deliveries {
		var notification AlertNotification
		if err := json.Unmarshal([]byte(deliveryPayloadFromDelivery(delivery)), &notification); err != nil {
			lastErr = err
			continue
		}
		if err := n.deliver(ctx, webhookNotifier, delivery, notification); err != nil {
			lastErr = err
			continue
		}
		retried++
	}
	return retried, lastErr
}

func (n *DynamicWebhookAlertNotifier) createDelivery(ctx context.Context, notification AlertNotification) (domain.AlertNotificationDelivery, error) {
	delivery, payload, ok, err := n.PrepareDelivery(ctx, notification)
	if err != nil {
		return domain.AlertNotificationDelivery{}, err
	}
	if !ok {
		return domain.AlertNotificationDelivery{}, nil
	}
	return n.deliveries.CreateNotificationDelivery(ctx, delivery, payload)
}

func (n *DynamicWebhookAlertNotifier) deliver(ctx context.Context, webhookNotifier *WebhookAlertNotifier, delivery domain.AlertNotificationDelivery, notification AlertNotification) error {
	now := time.Now().UTC()
	if err := webhookNotifier.NotifyAlert(ctx, notification); err != nil {
		nextAttemptAt := now.Add(nextAlertDeliveryDelay(delivery.AttemptCount + 1))
		recordErr := n.deliveries.RecordNotificationDeliveryAttempt(ctx, delivery.ID, domain.AlertNotificationDeliveryFailed, alertDeliveryErrorMessage(err), &nextAttemptAt, now)
		if recordErr != nil {
			return fmt.Errorf("%w; record alert notification delivery failure: %v", err, recordErr)
		}
		return err
	}
	if err := n.deliveries.RecordNotificationDeliveryAttempt(ctx, delivery.ID, domain.AlertNotificationDeliverySent, "", nil, now); err != nil {
		return fmt.Errorf("record alert notification delivery success: %w", err)
	}
	return nil
}

func nextAlertDeliveryDelay(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return time.Minute
	case attempt <= 3:
		return 5 * time.Minute
	case attempt <= 6:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

func alertDeliveryErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) <= 1024 {
		return message
	}
	return message[:1024]
}

func validateAlertWebhookHost(host string) error {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return fmt.Errorf("alert webhook url host is required")
	}
	if strings.EqualFold(host, "localhost") {
		if allowPrivateAlertWebhookHosts() {
			return nil
		}
		return fmt.Errorf("alert webhook url must not target localhost")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedAlertWebhookIP(ip) && !allowPrivateAlertWebhookHosts() {
			return fmt.Errorf("alert webhook url must not target private or loopback addresses")
		}
		return nil
	}
	return nil
}

func validateAlertWebhookURL(value *url.URL) error {
	if value == nil {
		return fmt.Errorf("alert webhook url is required")
	}
	if value.Scheme != "http" && value.Scheme != "https" {
		return fmt.Errorf("alert webhook url must use http or https")
	}
	if value.Host == "" {
		return fmt.Errorf("alert webhook url host is required")
	}
	if value.User != nil {
		return fmt.Errorf("alert webhook url must not include credentials")
	}
	return validateAlertWebhookHost(value.Hostname())
}

func newAlertWebhookHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		return dialAlertWebhookContext(ctx, dialer, network, address)
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("alert webhook redirect limit exceeded")
			}
			if err := validateAlertWebhookURL(req.URL); err != nil {
				return err
			}
			return nil
		},
	}
}

func dialAlertWebhookContext(ctx context.Context, dialer *net.Dialer, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := resolveAlertWebhookHost(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("alert webhook url host has no addresses")
}

var alertWebhookLookupIPAddr = net.DefaultResolver.LookupIPAddr

func resolveAlertWebhookHost(ctx context.Context, host string) ([]net.IP, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return nil, fmt.Errorf("alert webhook url host is required")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedAlertWebhookIP(ip) && !allowPrivateAlertWebhookHosts() {
			return nil, fmt.Errorf("alert webhook url must not target private or loopback addresses")
		}
		return []net.IP{ip}, nil
	}
	addresses, err := alertWebhookLookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve alert webhook url host: %w", err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("alert webhook url host has no addresses")
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		ip := address.IP
		if isBlockedAlertWebhookIP(ip) && !allowPrivateAlertWebhookHosts() {
			return nil, fmt.Errorf("alert webhook url must not resolve to private or loopback addresses")
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

func isBlockedAlertWebhookIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func allowPrivateAlertWebhookHosts() bool {
	value := strings.TrimSpace(os.Getenv("HOSTDECK_ALLOW_PRIVATE_WEBHOOK_HOSTS"))
	return value == "1" || strings.EqualFold(value, "true")
}

func deliveryPayloadFromDelivery(delivery domain.AlertNotificationDelivery) string {
	return delivery.Payload
}
