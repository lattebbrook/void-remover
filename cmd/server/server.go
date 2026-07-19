package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
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
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to parse file",
			})
		}

		fileExtension := strings.ToLower(filepath.Ext(file.Filename))

		if fileExtension == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "File extension is required",
			})
		}

		f, err := file.Open()

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to open file",
			})
		}

		defer f.Close()

		// 1. validate file size & length (max 10MB) & file type and extension (only png, jpg, jpeg)
		if !v.validateFile(f, file.Filename, fileExtension) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid file",
			})
		}

		// Normalize file name to avoid collisions and security issues
		file.Filename = uuid.New().String() + fileExtension

		filePath := filepath.Join(tempDir, file.Filename)
		err = c.SaveFile(file, filePath)

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to save file",
			})
		}

		// call lucida remove endpoint function to process the file
		fileProcessing(filePath)

		return c.JSON(fiber.Map{
			"message": "File uploaded successfully",
			"path":    filePath,
		})
	})

	// port
	app.Listen(":3000")
}
