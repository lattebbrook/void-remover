package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestResultDownloadHandler(t *testing.T) {
	const jobID = "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name          string
		path          string
		downloaderErr error
		downloadURL   string
		wantStatus    int
		wantCalled    bool
		wantLocation  string
	}{
		{
			name:       "invalid job id",
			path:       "/jobs/not-a-uuid/download",
			wantStatus: fiber.StatusBadRequest,
		},
		{
			name:          "result is not ready",
			path:          "/jobs/" + jobID + "/download",
			downloaderErr: errResultNotFound,
			wantStatus:    fiber.StatusNotFound,
			wantCalled:    true,
		},
		{
			name:          "s3 is unavailable",
			path:          "/jobs/" + jobID + "/download",
			downloaderErr: errors.New("s3 unavailable"),
			wantStatus:    fiber.StatusInternalServerError,
			wantCalled:    true,
		},
		{
			name:         "redirects to signed url",
			path:         "/jobs/" + jobID + "/download",
			downloadURL:  "https://signed-s3.example/results/image.png",
			wantStatus:   fiber.StatusFound,
			wantCalled:   true,
			wantLocation: "https://signed-s3.example/results/image.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloader := &fakeResultDownloader{
				downloadURL: tt.downloadURL,
				err:         tt.downloaderErr,
			}

			app := fiber.New()
			app.Get("/jobs/:id/download", resultDownloadHandler(downloader))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if downloader.called != tt.wantCalled {
				t.Errorf("downloader.called = %v, want %v", downloader.called, tt.wantCalled)
			}

			if got := resp.Header.Get("Location"); got != tt.wantLocation {
				t.Errorf("Location = %q, want %q", got, tt.wantLocation)
			}
		})
	}
}

type fakeResultDownloader struct {
	downloadURL string
	err         error
	called      bool
}

func (d *fakeResultDownloader) presignResultDownload(_ context.Context, jobID string) (string, error) {
	d.called = true
	if jobID != "550e8400-e29b-41d4-a716-446655440000" {
		return "", errors.New("unexpected job id")
	}
	return d.downloadURL, d.err
}
