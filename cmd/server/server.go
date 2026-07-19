package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/google/uuid"
)

// os environment variables
var tempDir string = os.Getenv("TEMP_DIR")
var lambdaEndpoint string = os.Getenv("LAMBDA_ENDPOINT")

// static variables
var basePath string = "/api/v1"

const (
	fileSizeLimit = 10 * 1024 * 1024 // 10 MB
	maxPixels     = 25_000_000       // 25 million pixels
)

type structValidator struct {
	validate *validator.Validate
}

func main() {
	app := fiber.New()

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
	app.Post(basePath+"/upload", func(c fiber.Ctx) error {
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

		// 1. validate file size & length (max 10MB) & file type and extension (only png, jpg, jpeg)
		cleanedData := v.validateFile(f, file.Filename, fileExtension)

		id := uuid.New().String()

		originalPath := filepath.Join(tempDir, id+"-original"+fileExtension)
		cleanedPath := filepath.Join(tempDir, id+"-cleaned"+fileExtension)

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

		//fileProcessing(cleanedPath)

		defer func() {
			// Clean up the temporary files after processing.
			os.Remove(originalPath)
			os.Remove(cleanedPath)
		}()

		return c.JSON(fiber.Map{
			"message": "File uploaded successfully",
			"path":    cleanedPath,
		})
	})

	// port
	app.Listen(":3000")
}
