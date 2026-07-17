// MediaClient streams KYC document/selfie uploads from the app to
// media-service (POST /api/v1/media/upload) and returns the stored media
// metadata, whose id becomes the documentRef/selfieRef on an onboarding
// submission. Like ComplianceClient it calls the service DIRECTLY with
// X-Service-Key auth (the public gateway strips service-auth headers).
//
// The BFF existing surface had no media proxy (media-service was only used by
// the staff portal via the lms-api-gateway), so this is the first app-side
// media path.
package client

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/athena-lms/go-services/internal/common/auth"
)

type MediaClient struct {
	baseURL string
	client  *http.Client
}

func NewMediaClient(baseURL, serviceKey string) *MediaClient {
	return &MediaClient{
		baseURL: baseURL,
		client: &http.Client{
			Transport: &auth.ServiceKeyTransport{
				ServiceKey:  serviceKey,
				ServiceName: "mobile-gateway",
			},
		},
	}
}

// MediaUpload describes one file to forward to media-service.
type MediaUpload struct {
	// FileName and ContentType come from the app's multipart part.
	FileName    string
	ContentType string
	// MediaType is a media-service MediaType (ID_FRONT, SELFIE, ...).
	MediaType string
	// Category is a media-service MediaCategory (CUSTOMER_DOCUMENT, ...).
	Category string
	// File is the upload body; it is streamed, never buffered whole.
	File io.Reader
}

// Upload streams the file to media-service as multipart/form-data via an
// io.Pipe (no full in-memory copy on the BFF side) and returns the media
// metadata map (including "id").
func (c *MediaClient) Upload(ctx context.Context, up MediaUpload) (map[string]any, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		err := writeUploadForm(mw, up)
		if cerr := mw.Close(); err == nil {
			err = cerr
		}
		pw.CloseWithError(err) // nil err closes cleanly
	}()

	url := fmt.Sprintf("%s/api/v1/media/upload", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return doProxy(c.client, "media", req)
}

// writeUploadForm writes the media-service upload form: the required
// category/mediaType fields, provenance fields, then the streamed file part
// with its original content type preserved.
func writeUploadForm(mw *multipart.Writer, up MediaUpload) error {
	fields := map[string]string{
		"category":    up.Category,
		"mediaType":   up.MediaType,
		"serviceName": "bff-gateway",
		"channel":     "MOBILE",
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return err
		}
	}
	hdr := textproto.MIMEHeader{}
	hdr.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(up.FileName)))
	contentType := up.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	hdr.Set("Content-Type", contentType)
	part, err := mw.CreatePart(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, up.File)
	return err
}

// escapeQuotes mirrors mime/multipart's unexported quote escaping.
func escapeQuotes(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}
