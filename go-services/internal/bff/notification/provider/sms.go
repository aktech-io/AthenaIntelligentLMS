package provider

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type SmsProvider struct {
	apiKey   string
	username string
	senderID string
}

func NewSmsProvider(apiKey, username, senderID string) *SmsProvider {
	return &SmsProvider{apiKey: apiKey, username: username, senderID: senderID}
}

func (p *SmsProvider) SendSms(to, message string) (string, error) {
	if p.username == "sandbox" {
		slog.Info("sandbox SMS", "to", to, "message", message)
		return "sandbox-" + uuid.New().String(), nil
	}

	apiURL := "https://api.africastalking.com/version1/messaging"

	form := url.Values{}
	form.Set("username", p.username)
	form.Set("to", to)
	form.Set("message", message)
	if p.senderID != "" {
		form.Set("from", p.senderID)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create sms request: %w", err)
	}
	req.Header.Set("apiKey", p.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send sms: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("sms api error %d: %s", resp.StatusCode, string(body))
	}

	slog.Info("SMS sent", "to", to, "status", resp.StatusCode)
	return string(body), nil
}
