package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
)

func (v *structValidator) validateFile(file multipart.File, fileName string, fileExtension string) bool {
	data, err := io.ReadAll(io.LimitReader(file, fileSizeLimit+1))

	if err != nil {
		panic("50000") // internal server error or failed to read file
	}

	// 1 check size of file 10 MB max
	if len(data) > fileSizeLimit {
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

	// Check the actual bytes, not filename or request header.
	mimeType := http.DetectContentType(data)
	switch mimeType {
	case "image/png", "image/jpeg":
		// Allowed.
	default:
		panic("41000") // bad request invalid file type
	}

	// Parse image metadata without allocating the entire decoded image yet.
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		panic("41000") // bad request invalid file type
	}

	if format != "png" && format != "jpeg" {
		panic("41000") // bad request invalid file type
	}

	// Ensure MIME sniffing and the decoder agree.
	if mimeType == "image/png" && format != "png" {
		panic("41000") // bad request invalid file type
	}

	if mimeType == "image/jpeg" && format != "jpeg" {
		panic("41000") // bad request invalid file type
	}

	if config.Width <= 0 || config.Height <= 0 {
		panic("41000") // bad request invalid image dimensions
	}

	pixelCount := uint64(config.Width) * uint64(config.Height)

	if pixelCount > maxPixels {
		panic("41000") // bad request image dimensions too large
	}

	// Fully decode to catch corrupted or truncated image data.
	decodedImage, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		panic("41000") // bad request invalid file type
	}

	// Re-encode the image. This strips trailing payloads and most metadata.
	var cleaned bytes.Buffer

	switch decodedFormat {
	case "png":
		err = png.Encode(&cleaned, decodedImage)

	case "jpeg":
		err = jpeg.Encode(&cleaned, decodedImage, &jpeg.Options{
			Quality: 95,
		})

	default:
		panic("41000") // bad request invalid file type
	}

	if err != nil {
		panic("50000") // internal server error or failed to encode image
	}

	return true
}
