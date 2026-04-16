package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// AlertChannel defines a notification target.
type AlertChannel struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "email" | "slack" | "teams" | "webhook"
	Enabled  bool   `json:"enabled"`
	// Email
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	SMTPUser string `json:"smtp_user"`
	SMTPPass string `json:"smtp_pass"`
	FromAddr string `json:"from_addr"`
	ToAddrs  []string `json:"to_addrs"`
	// Slack / Teams / Webhook
	WebhookURL string `json:"webhook_url"`
	// Filtering
	MinSeverity string `json:"min_severity"` // "INFO" | "WARN" | "ERROR"
}

// AlertNotification is the payload sent to a channel.
type AlertNotification struct {
	Severity  string
	Server    string
	Title     string
	Body      string
	Timestamp time.Time
}

// Notifier dispatches alerts to all configured channels with deduplication.
type Notifier struct {
	channels []AlertChannel
	dedup    *AlertDeduplicator
	mu       sync.Mutex
	logger   *Logger
}

// NewNotifier creates a Notifier wired to the configured alert channels.
func NewNotifier(channels []AlertChannel, cooldownMin int, logger *Logger) *Notifier {
	return &Notifier{
		channels: channels,
		dedup:    NewAlertDeduplicator(cooldownMin),
		logger:   logger,
	}
}

// Send dispatches an alert to all matching enabled channels.
func (n *Notifier) Send(sev, server, title, body string) {
	if !n.dedup.ShouldAlert(server, title) {
		return
	}
	notif := AlertNotification{
		Severity:  sev,
		Server:    server,
		Title:     title,
		Body:      body,
		Timestamp: time.Now(),
	}
	for _, ch := range n.channels {
		if !ch.Enabled {
			continue
		}
		if !severityMeets(sev, ch.MinSeverity) {
			continue
		}
		ch := ch // capture
		go func() {
			var err error
			switch ch.Type {
			case "email":
				err = sendEmail(ch, notif)
			case "slack":
				err = sendSlack(ch, notif)
			case "teams":
				err = sendTeams(ch, notif)
			case "webhook":
				err = sendWebhook(ch, notif)
			}
			if err != nil {
				n.logger.Error("", fmt.Sprintf("Alert channel [%s] failed: %v", ch.Name, err))
			} else {
				n.logger.Debug("", fmt.Sprintf("Alert sent via [%s] channel: %s", ch.Name, title))
			}
		}()
	}
}

func severityMeets(have, need string) bool {
	rank := map[string]int{"DEBUG": 0, "INFO": 1, "WARN": 2, "ERROR": 3}
	return rank[strings.ToUpper(have)] >= rank[strings.ToUpper(need)]
}

// ── Email ─────────────────────────────────────────────────────────────────────

func sendEmail(ch AlertChannel, n AlertNotification) error {
	if ch.SMTPHost == "" || len(ch.ToAddrs) == 0 {
		return fmt.Errorf("email channel missing smtp_host or to_addrs")
	}
	port := ch.SMTPPort
	if port == 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", ch.SMTPHost, port)
	from := ch.FromAddr
	if from == "" {
		from = ch.SMTPUser
	}

	subject := fmt.Sprintf("[%s] %s — %s", n.Severity, n.Server, n.Title)
	body := fmt.Sprintf(
		"DBLens SQL Monitor Alert\r\n"+
			"========================\r\n"+
			"Server:    %s\r\n"+
			"Severity:  %s\r\n"+
			"Time:      %s\r\n"+
			"Alert:     %s\r\n\r\n"+
			"%s\r\n",
		n.Server, n.Severity, n.Timestamp.Format("2006-01-02 15:04:05"), n.Title, n.Body,
	)
	msg := []byte("To: " + strings.Join(ch.ToAddrs, ",") + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		body)

	var auth smtp.Auth
	if ch.SMTPUser != "" && ch.SMTPPass != "" {
		auth = smtp.PlainAuth("", ch.SMTPUser, ch.SMTPPass, ch.SMTPHost)
	}
	return smtp.SendMail(addr, auth, from, ch.ToAddrs, msg)
}

// ── Slack ─────────────────────────────────────────────────────────────────────

func sendSlack(ch AlertChannel, n AlertNotification) error {
	if ch.WebhookURL == "" {
		return fmt.Errorf("slack channel missing webhook_url")
	}
	emoji := map[string]string{"ERROR": "🔴", "WARN": "🟡", "INFO": "🔵"}[n.Severity]
	if emoji == "" {
		emoji = "⚪"
	}
	payload := map[string]interface{}{
		"text": fmt.Sprintf("%s *[%s] %s* — %s", emoji, n.Severity, n.Server, n.Title),
		"attachments": []map[string]interface{}{
			{
				"color": map[string]string{"ERROR": "#ef4444", "WARN": "#f59e0b", "INFO": "#3b82f6"}[n.Severity],
				"fields": []map[string]string{
					{"title": "Server", "value": n.Server, "short": "true"},
					{"title": "Severity", "value": n.Severity, "short": "true"},
					{"title": "Time", "value": n.Timestamp.Format("15:04:05"), "short": "true"},
				},
				"text":   n.Body,
				"footer": "DBLens Monitor",
			},
		},
	}
	return postJSON(ch.WebhookURL, payload)
}

// ── Microsoft Teams ───────────────────────────────────────────────────────────

func sendTeams(ch AlertChannel, n AlertNotification) error {
	if ch.WebhookURL == "" {
		return fmt.Errorf("teams channel missing webhook_url")
	}
	color := map[string]string{"ERROR": "attention", "WARN": "warning", "INFO": "accent"}[n.Severity]
	if color == "" {
		color = "default"
	}
	payload := map[string]interface{}{
		"type":    "message",
		"attachments": []map[string]interface{}{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]interface{}{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body": []map[string]interface{}{
						{"type": "TextBlock", "text": fmt.Sprintf("**DBLens Alert — %s**", n.Server), "weight": "Bolder", "size": "Medium", "color": color},
						{"type": "TextBlock", "text": n.Title, "wrap": true},
						{"type": "FactSet", "facts": []map[string]string{
							{"title": "Server", "value": n.Server},
							{"title": "Severity", "value": n.Severity},
							{"title": "Time", "value": n.Timestamp.Format("2006-01-02 15:04:05")},
						}},
						{"type": "TextBlock", "text": n.Body, "wrap": true, "isSubtle": true},
					},
				},
			},
		},
	}
	return postJSON(ch.WebhookURL, payload)
}

// ── Generic Webhook ───────────────────────────────────────────────────────────

func sendWebhook(ch AlertChannel, n AlertNotification) error {
	if ch.WebhookURL == "" {
		return fmt.Errorf("webhook channel missing webhook_url")
	}
	payload := map[string]interface{}{
		"severity":  n.Severity,
		"server":    n.Server,
		"title":     n.Title,
		"body":      n.Body,
		"timestamp": n.Timestamp.Format(time.RFC3339),
		"source":    "dblens-monitor",
	}
	return postJSON(ch.WebhookURL, payload)
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func postJSON(url string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
