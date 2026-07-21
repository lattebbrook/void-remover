package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fileProcessor struct {
	s3Client     *s3.Client
	lambdaClient *awslambda.Client
	bucket       string
	functionName string
}

type lucidaEvent struct {
	InputBucket  string `json:"input_bucket"`
	InputKey     string `json:"input_key"`
	OutputBucket string `json:"output_bucket"`
	OutputKey    string `json:"output_key"`
}

func newFileProcessor(ctx context.Context) (*fileProcessor, error) {
	bucket := strings.TrimSpace(os.Getenv("S3_BUCKET"))
	if bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required")
	}

	functionName := strings.TrimSpace(os.Getenv("LAMBDA_FUNCTION_NAME"))
	if functionName == "" {
		return nil, fmt.Errorf("LAMBDA_FUNCTION_NAME is required")
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	if cfg.Region == "" {
		return nil, fmt.Errorf("AWS_REGION is required")
	}

	return &fileProcessor{
		s3Client:     s3.NewFromConfig(cfg),
		lambdaClient: awslambda.NewFromConfig(cfg),
		bucket:       bucket,
		functionName: functionName,
	}, nil
}

func (p *fileProcessor) process(
	ctx context.Context,
	filePath string,
	jobID string,
	fileExtension string,
) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open cleaned file: %w", err)
	}
	defer file.Close()

	fileExtension = strings.ToLower(fileExtension)

	var contentType string
	switch fileExtension {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	default:
		return "", fmt.Errorf("unsupported file extension %q", fileExtension)
	}

	inputKey := "upload/" + jobID + fileExtension
	outputKey := "results/" + jobID + ".png"

	_, err = p.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(p.bucket),
		Key:         aws.String(inputKey),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("upload cleaned image to S3: %w", err)
	}

	event := lucidaEvent{
		InputBucket:  p.bucket,
		InputKey:     inputKey,
		OutputBucket: p.bucket,
		OutputKey:    outputKey,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal Lambda event: %w", err)
	}

	result, err := p.lambdaClient.Invoke(ctx, &awslambda.InvokeInput{
		FunctionName:   aws.String(p.functionName),
		InvocationType: lambdatypes.InvocationTypeEvent,
		Payload:        payload,
	})
	if err != nil {
		return "", fmt.Errorf("invoke Lucida Lambda: %w", err)
	}

	if result.StatusCode != 202 {
		return "", fmt.Errorf(
			"Lambda rejected invocation with status %d",
			result.StatusCode,
		)
	}

	return outputKey, nil
}
