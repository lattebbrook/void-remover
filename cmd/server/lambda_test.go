package main

import (
	"errors"
	"testing"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func TestResultKeyForJob(t *testing.T) {
	const jobID = "550e8400-e29b-41d4-a716-446655440000"
	const want = "results/550e8400-e29b-41d4-a716-446655440000.png"

	if got := resultKeyForJob(jobID); got != want {
		t.Errorf("resultKeyForJob() = %q, want %q", got, want)
	}
}

func TestIsS3NotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "modeled S3 not found error",
			err:  &s3types.NotFound{},
			want: true,
		},
		{
			name: "generic no such key error",
			err:  &smithy.GenericAPIError{Code: "NoSuchKey"},
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("network failure"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isS3NotFound(tt.err); got != tt.want {
				t.Errorf("isS3NotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}
