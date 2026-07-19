# VoidRemover Background Remover

A simple background-removal demo built with **Go**, **AWS Lambda**, and the [Lucida](https://github.com/egeorcun/lucida) image-matting model.

Users can upload a photo and receive a transparent PNG with the background removed.

## How It Works

1. The user uploads an image through the web interface.
2. The Go API validates the file and sends it to AWS Lambda.
3. Lambda processes the image using Lucida.
4. The processed transparent PNG is returned to the user.

## Tech Stack

- Go
- AWS Lambda
- Python
- Lucida
- HTML, CSS and JavaScript

## Features

- Drag-and-drop image upload
- AI-powered background removal
- Before-and-after preview
- Transparent PNG download
- Dark-mode interface

## Project Purpose

This project demonstrates how Go can coordinate with a serverless Python image-processing service to provide AI-powered background removal through a simple web application.
