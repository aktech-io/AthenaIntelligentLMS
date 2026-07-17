package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/athena-lms/go-services/internal/common/auth"
	apperrors "github.com/athena-lms/go-services/internal/common/errors"
)

// TestMediaUploadStreamsMultipart — the client re-encodes the upload as the
// media-service form contract (file + category/mediaType/provenance fields),
// preserving the file's content type, and returns the media metadata.
func TestMediaUploadStreamsMultipart(t *testing.T) {
	type got struct {
		fields      map[string]string
		fileName    string
		contentType string
		content     string
		tenant      string
	}
	var g got
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/media/upload", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(1<<20))
		g.fields = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			g.fields[k] = v[0]
		}
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()
		content, _ := io.ReadAll(file)
		g.fileName = header.Filename
		g.contentType = header.Header.Get("Content-Type")
		g.content = string(content)
		g.tenant = r.Header.Get("X-Service-Tenant")

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"media-123","category":"CUSTOMER_DOCUMENT"}`))
	}))
	defer srv.Close()

	c := NewMediaClient(srv.URL, "sk-test")
	ctx := auth.WithTenantID(context.Background(), "acme")
	resp, err := c.Upload(ctx, MediaUpload{
		FileName:    "id-front.jpg",
		ContentType: "image/jpeg",
		MediaType:   "ID_FRONT",
		Category:    "CUSTOMER_DOCUMENT",
		File:        strings.NewReader("fake-jpeg-bytes"),
	})

	require.NoError(t, err)
	assert.Equal(t, "media-123", resp["id"])
	assert.Equal(t, "CUSTOMER_DOCUMENT", g.fields["category"])
	assert.Equal(t, "ID_FRONT", g.fields["mediaType"])
	assert.Equal(t, "bff-gateway", g.fields["serviceName"])
	assert.Equal(t, "MOBILE", g.fields["channel"])
	assert.Equal(t, "id-front.jpg", g.fileName)
	assert.Equal(t, "image/jpeg", g.contentType)
	assert.Equal(t, "fake-jpeg-bytes", g.content)
	assert.Equal(t, "acme", g.tenant)
}

// TestMediaUploadServiceDown — media-service unreachable surfaces as 503.
func TestMediaUploadServiceDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	c := NewMediaClient(srv.URL, "sk-test")
	_, err := c.Upload(context.Background(), MediaUpload{
		FileName: "x.jpg", MediaType: "SELFIE", Category: "CUSTOMER_DOCUMENT",
		File: strings.NewReader("x"),
	})

	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, http.StatusServiceUnavailable, be.StatusCode)
}

// TestMediaUpload5xxMapsTo502 — a media-service 500 surfaces as 502.
func TestMediaUpload5xxMapsTo502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "disk full", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewMediaClient(srv.URL, "sk-test")
	_, err := c.Upload(context.Background(), MediaUpload{
		FileName: "x.jpg", MediaType: "SELFIE", Category: "CUSTOMER_DOCUMENT",
		File: strings.NewReader("x"),
	})

	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, http.StatusBadGateway, be.StatusCode)
}
