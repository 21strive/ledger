package ledger

import (
	"context"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/21strive/ledger/ledgererr"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (c *LedgerClient) GetPhotoKTPPresignedURL(ctx context.Context, sellerID string, bucketName string, contentType string) (string, error) {
	validatedExt, err := validateContentType(contentType)
	if err != nil {
		return "", ledgererr.ErrInvalidRequest.WithError(err)
	}
	// Normalize the sellerID to be URL-safe
	normalizedSellerID := strings.ReplaceAll(sellerID, " ", "-")

	key := fmt.Sprintf("verification/kyc/%s/kyc-selfie.%s", normalizedSellerID, validatedExt)

	presignClient := s3.NewPresignClient(c.s3)
	presignResult, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:  aws.String(bucketName),
		Key:     aws.String(key),
		Expires: aws.Time(time.Now().Add(15 * time.Minute)),
	})

	if err != nil {
		return "", ledgererr.ErrInvalidRequest.WithError(err)
	}

	return presignResult.URL, nil
}

func (c *LedgerClient) GetPhotoKYCSelfiePresignedURL(ctx context.Context, sellerID string, bucketName string, contentType string) (string, error) {
	validatedExt, err := validateContentType(contentType)
	if err != nil {
		return "", err
	}
	// Normalize the sellerID to be URL-safe
	normalizedSellerID := strings.ReplaceAll(sellerID, " ", "-")

	key := fmt.Sprintf("verification/kyc/%s/kyc-selfie.%s", normalizedSellerID, validatedExt)

	presignClient := s3.NewPresignClient(c.s3)
	presignResult, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		ContentType: aws.String(validatedExt),
		Expires:     aws.Time(time.Now().Add(15 * time.Minute)),
	})

	if err != nil {
		return "", err
	}

	return presignResult.URL, nil
}

// Only accepts JPEG and PNG
func validateContentType(contentType string) (string, error) {
	// Only allow images
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unsupported file type: %s", contentType)
	}

	exts, err := mime.ExtensionsByType(contentType)
	if err != nil {
		return "", err
	} else if len(exts) == 0 {
		return "", fmt.Errorf("extensions for content type: %s is empty", contentType)
	}

	ext := strings.TrimPrefix(contentType, "image/")
	if ext != "jpeg" && ext != "jpg" && ext != "png" {
		return "", fmt.Errorf("unsupported image type: %s [only JPEG and PNG are allowed]", contentType)
	}

	return ext, nil
}
