package main

import (
	"strings"

	"os"

	"github.com/gofiber/fiber/v3"
)

var basePath string = "/api/v1"
var tempDir string = os.Getenv("TEMP_DIR")

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
		file, err := c.FormFile("file")

		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to parse file",
			})
		}

		// 1.0 validate file extension first (only accept png, jpg, jpeg)
		allowedExtensions := map[string]bool{
			".png":  true,
			".jpg":  true,
			".jpeg": true,
		}

		fileExtension := strings.ToLower(file.Filename[strings.LastIndex(file.Filename, "."):])

		if !allowedExtensions[fileExtension] {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid file extension",
			})
		}

		// TODO: rename the file to a unique name to avoid conflicts

		filePath := "./temp/" + file.Filename
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

func processFile(filePath string) {
	// trigger call to external python script to process the file

}
