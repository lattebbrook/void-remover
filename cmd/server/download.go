package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	downloadURLTTL     = time.Minute
	downloadCheckLimit = 10 * time.Second
)

var errResultNotFound = errors.New("result not found")

type resultDownloader interface {
	presignResultDownload(ctx context.Context, jobID string) (string, error)
}

func resultDownloadHandler(downloader resultDownloader) fiber.Handler {
	return func(c fiber.Ctx) error {
		jobID, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "INVALID_JOB_ID",
			})
		}

		ctx, cancel := context.WithTimeout(c.Context(), downloadCheckLimit)
		defer cancel()

		downloadURL, err := downloader.presignResultDownload(ctx, jobID.String())
		if err != nil {
			if errors.Is(err, errResultNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
					"error": "RESULT_NOT_FOUND",
				})
			}

			log.Printf("Failed to prepare result download job_id=%s: %v", jobID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "RESULT_DOWNLOAD_UNAVAILABLE",
			})
		}

		return c.Redirect().Status(fiber.StatusFound).To(downloadURL)
	}
}
