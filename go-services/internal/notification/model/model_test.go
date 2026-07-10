package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func intPtr(i int) *int { return &i }

func TestNotificationConfig_JSONRoundTrip(t *testing.T) {
	cfg := NotificationConfig{
		ID:          1,
		Type:        "EMAIL",
		Provider:    strPtr("SMTP"),
		Host:        strPtr("smtp.gmail.com"),
		Port:        intPtr(587),
		Username:    strPtr("user@gmail.com"),
		Password:    strPtr("secret"),
		FromAddress: strPtr("noreply@athena.co.ke"),
		Enabled:     true,
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	var decoded NotificationConfig
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, cfg.Type, decoded.Type)
	assert.Equal(t, cfg.Host, decoded.Host)
	assert.Equal(t, cfg.Port, decoded.Port)
	assert.Equal(t, cfg.FromAddress, decoded.FromAddress)
	assert.Equal(t, cfg.Enabled, decoded.Enabled)
}

func TestNotificationConfig_JSONFieldNames(t *testing.T) {
	cfg := NotificationConfig{
		FromAddress: strPtr("test@example.com"),
		APIKey:      strPtr("key123"),
		APISecret:   strPtr("secret456"),
		SenderID:    strPtr("ATHENA"),
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// Verify camelCase JSON field names match Java DTO
	assert.Equal(t, "test@example.com", raw["fromAddress"])
	assert.Equal(t, "key123", raw["apiKey"])
	assert.Equal(t, "secret456", raw["apiSecret"])
	assert.Equal(t, "ATHENA", raw["senderId"])
}

func TestNotificationLog_JSONRoundTrip(t *testing.T) {
	log := NotificationLog{
		ID:           42,
		ServiceName:  "payment-service",
		Type:         "EMAIL",
		Recipient:    "customer@example.com",
		Subject:      "Payment Confirmed",
		Body:         "Your payment has been received.",
		Status:       "SENT",
		ErrorMessage: "",
	}

	data, err := json.Marshal(log)
	require.NoError(t, err)

	var decoded NotificationLog
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, log.ServiceName, decoded.ServiceName)
	assert.Equal(t, log.Recipient, decoded.Recipient)
	assert.Equal(t, log.Status, decoded.Status)
}

func TestNotificationLog_ErrorMessageOmittedWhenEmpty(t *testing.T) {
	log := NotificationLog{
		Status:       "SENT",
		ErrorMessage: "",
	}

	data, err := json.Marshal(log)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// errorMessage has omitempty, so empty string should be omitted
	_, exists := raw["errorMessage"]
	assert.False(t, exists, "errorMessage should be omitted when empty")
}

func TestNotificationRequest_JSONDecode(t *testing.T) {
	jsonStr := `{
		"serviceName": "scoring-service",
		"type": "EMAIL",
		"recipient": "user@example.com",
		"subject": "Score Updated",
		"message": "Your score is 750",
		"metadata": {"score": 750}
	}`

	var req NotificationRequest
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &req))

	assert.Equal(t, "scoring-service", req.ServiceName)
	assert.Equal(t, "EMAIL", req.Type)
	assert.Equal(t, "user@example.com", req.Recipient)
	assert.Equal(t, "Score Updated", req.Subject)
	assert.Equal(t, "Your score is 750", req.Message)
	assert.NotNil(t, req.Metadata)
	assert.Equal(t, float64(750), req.Metadata["score"])
}
