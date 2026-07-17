package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type NotificationClient struct {
	baseURL string
	client  *http.Client
}

func NewNotificationClient(baseURL, serviceKey string) *NotificationClient {
	return &NotificationClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

func (c *NotificationClient) SendOtp(ctx context.Context, phoneNumber, otp string) error {
	url := fmt.Sprintf("%s/api/v1/notifications/sms/otp", c.baseURL)
	_, err := doJSONPost(ctx, c.client, url, map[string]string{
		"phoneNumber": phoneNumber,
		"otp":         otp,
	})
	return err
}

func (c *NotificationClient) SendNotification(ctx context.Context, body map[string]any) error {
	url := fmt.Sprintf("%s/api/v1/notifications/send", c.baseURL)
	_, err := doJSONPost(ctx, c.client, url, body)
	return err
}

func (c *NotificationClient) GetUnreadCount(ctx context.Context, userID string) (int64, error) {
	url := fmt.Sprintf("%s/api/v1/notifications/user/%s/unread-count", c.baseURL, userID)
	result, err := doJSONGet(ctx, c.client, url)
	if err != nil {
		return 0, err
	}
	count, _ := result["unreadCount"].(float64)
	return int64(count), nil
}
