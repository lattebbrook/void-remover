package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type fileProcessor struct {
	s3Client        *s3.Client
	s3PresignClient *s3.PresignClient
	lambdaClient    *awslambda.Client
	bucket          string
	functionName    string
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

	s3Client := s3.NewFromConfig(cfg)

	return &fileProcessor{
		s3Client:        s3Client,
		s3PresignClient: s3.NewPresignClient(s3Client),
		lambdaClient:    awslambda.NewFromConfig(cfg),
		bucket:          bucket,
		functionName:    functionName,
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
	outputKey := resultKeyForJob(jobID)

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

func (p *fileProcessor) presignResultDownload(ctx context.Context, jobID string) (string, error) {
	resultKey := resultKeyForJob(jobID)

	_, err := p.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(resultKey),
	})
	if err != nil {
		if isS3NotFound(err) {
			return "", errResultNotFound
		}
		return "", fmt.Errorf("check result object: %w", err)
	}

	presigned, err := p.s3PresignClient.PresignGetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket:                     aws.String(p.bucket),
			Key:                        aws.String(resultKey),
			ResponseContentDisposition: aws.String(fmt.Sprintf("attachment; filename=\"void-remover-%s.png\"", jobID)),
			ResponseContentType:        aws.String("image/png"),
		},
		func(options *s3.PresignOptions) {
			options.Expires = downloadURLTTL
		},
	)
	if err != nil {
		return "", fmt.Errorf("presign result download: %w", err)
	}

	return presigned.URL, nil
}

func resultKeyForJob(jobID string) string {
	return "results/" + jobID + ".png"
}

func isS3NotFound(err error) bool {
	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchObject":
			return true
		}
	}

	return false
}
