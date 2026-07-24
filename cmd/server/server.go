package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/google/uuid"
)

// os environment variables
var tempDir string = os.Getenv("TEMP_DIR")

// static variables
var basePath string = "/api/v1"

const (
	fileSizeLimit      = 4 * 1024 * 1024 // 4 MB
	multipartBodyLimit = fileSizeLimit + 1024*1024
	maxPixels          = 25_000_000 // 25 million pixels
)

type structValidator struct {
	validate *validator.Validate
}

func main() {
	processor, err := newFileProcessor(context.Background())
	if err != nil {
		log.Fatal("Failed to initialize AWS clients:", err)
	}

	turnstile, err := newTurnstileVerifier()
	if err != nil {
		log.Fatal("Failed to initialize Turnstile verifier:", err)
	}

	app := fiber.New(fiber.Config{
		// Allow room for multipart headers while validateFile enforces the
		// actual 4 MB limit on the uploaded image bytes.
		BodyLimit: multipartBodyLimit,
	})

	// Initialize default config
	app.Use(logger.New())
	// Logging Request ID
	app.Use(requestid.New()) // Ensure requestid middleware is used before the logger
	app.Use(logger.New(logger.Config{
		// requestid.New() registers ${requestid} automatically.
		Format: "${pid} ${requestid} ${status} - ${method} ${path}\n",
	}))

	// Changing TimeZone & TimeFormat
	app.Use(logger.New(logger.Config{
		Format:     "${pid} ${status} - ${method} ${path}\n",
		TimeFormat: "02-Jan-2006",
		TimeZone:   "Asia/Bangkok",
	}))

	app.Use(recover.New(recover.Config{
		PanicHandler: func(c fiber.Ctx, recovered any) error {
			code := fmt.Sprint(recovered)

			status := fiber.StatusInternalServerError

			switch code {
			case "41000", "42000":
				status = fiber.StatusBadRequest
			case "43000":
				status = fiber.StatusRequestEntityTooLarge
			case "50000":
				status = fiber.StatusInternalServerError
			default:
				code = "50000"
			}

			return c.Status(status).JSON(fiber.Map{
				"error": code,
			})
		},
	}))

	// @@ 1. Main route
	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("Welcome to the API")
	})

	// @@ 2. Health check route
	app.Get(basePath+"/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "up",
		})
	})

	v := &structValidator{
		validate: validator.New(),
	}

	// @@ 3. Upload route
	app.Post(basePath+"/upload", requireTurnstile(turnstile), func(c fiber.Ctx) error {
		file, err := c.FormFile("file")

		if err != nil {
			log.Println("Error retrieving the file:", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to parse file",
			})
		}

		fileExtension := strings.ToLower(filepath.Ext(file.Filename))

		if fileExtension == "" {
			log.Println(file.Filename + ": File extension is required")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "File extension is required",
			})
		}

		f, err := file.Open()

		if err != nil {
			log.Println("Error opening file:", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to open file",
			})
		}

		defer f.Close()

		// 1. validate file size & length (max 4MB) & file type and extension (only png, jpg, jpeg)
		cleanedData := v.validateFile(f, file.Filename, fileExtension)

		id := uuid.New().String()

		originalPath := filepath.Join(tempDir, id+"-original"+fileExtension)
		cleanedPath := filepath.Join(tempDir, id+"-cleaned"+fileExtension)

		defer func() {
			// Clean up the temporary files after the S3 upload has completed.
			os.Remove(originalPath)
			os.Remove(cleanedPath)
		}()

		// Preserve the customer's original upload.
		if err := c.SaveFile(file, originalPath); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to save original file",
			})
		}

		// Save the sanitized version for Lambda processing.
		if err := os.WriteFile(cleanedPath, cleanedData, 0600); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to save cleaned file",
			})
		}

		processCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		outputKey, err := processor.process(
			processCtx,
			cleanedPath,
			id,
			fileExtension,
		)
		if err != nil {
			log.Printf("Failed to submit image job id=%s: %v", id, err)
			panic("50000")
		}

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"message":    "Image processing started",
			"job_id":     id,
			"result_key": outputKey,
		})
	})

	// @@ 4. Download a completed result through a short-lived S3 URL.
	app.Get(basePath+"/jobs/:id/download", resultDownloadHandler(processor))

	// port
	app.Listen(":3000")
}
