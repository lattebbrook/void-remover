package main

import (
	"mime/multipart"
	"strings"

	"os"

	"github.com/google/uuid"

	"github.com/gofiber/fiber/v3"
)

// os environment variables
var tempDir string = os.Getenv("TEMP_DIR")
var lambdaEndpoint string = os.Getenv("LAMBDA_ENDPOINT")

// static variables
var basePath string = "/api/v1"
var fileSizeLimit int64 = 10 * 1024 * 1024 // 10MB

func main() {
	app := fiber.New()

	app.Get("/", func(c fiber.Ctx) error {
		return c.SendString("Welcome to the API")
	})

	app.Get(basePath+"/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "up",
		})
	})

	app.Post(basePath+"/upload/:file", func(c fiber.Ctx) error {
		// prepare to add validator for header and body

		file, err := c.FormFile("file")

		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to parse file",
			})
		}

		fileExtension := strings.ToLower(file.Filename[strings.LastIndex(file.Filename, "."):])

		// 1. validate file size & length (max 10MB)
		if !validateFile(file.Filename, file, fileExtension) { // 10MB
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// Normalize file name to avoid collisions and security issues
		file.Filename = uuid.New().String() + fileExtension

		filePath := tempDir + file.Filename
		err = c.SaveFile(file, filePath)

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to save file",
			})
		}

		// call lucida remove endpoint function to process the file
		// processFile(filePath)

		return c.JSON(fiber.Map{
			"message": "File uploaded successfully",
			"path":    filePath,
		})
	})

	app.Listen(":3000")
}

// TODO: change to validation v10
func validateFile(fileName string, file *multipart.FileHeader, fileExtension string) bool {
	// 1 check size of file 10 MB max
	if file.Size > fileSizeLimit {
		panic("43000") // payload too large > 10MB
	}

	// 2 check length of file name
	if len(fileName) > 255 {
		panic("42000") // bad request length of file name too long
	}

	// 2. validate file extension first (only accept png, jpg, jpeg)
	allowedExtensions := map[string]bool{
		".png":  true,
		".jpg":  true,
		".jpeg": true,
	}

	if !allowedExtensions[fileExtension] {
		panic("41000") // bad request invalid file extension
	}

	return true
}
