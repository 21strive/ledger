package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (c *LedgerClient) GetPhotoKTPPresignedURL(ctx context.Context, sellerID string, bucketName string) (string, error) {
	presignClient := s3.NewPresignClient(c.s3)
	// Normalize the sellerID to be URL-safe
	normalizedSellerID := strings.ReplaceAll(sellerID, " ", "-")

	key := fmt.Sprintf("verification/ktp/%s/ktp.jpg", normalizedSellerID)
	presignResult, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:  aws.String(bucketName),
		Key:     aws.String(key),
		Expires: aws.Time(time.Now().Add(15 * time.Minute)),
	})

	if err != nil {
		return "", err
	}

	return presignResult.URL, nil
}

func (c *LedgerClient) GetPhotoKYCSelfiePresignedURL(ctx context.Context, sellerID string, bucketName string) (string, error) {
	presignClient := s3.NewPresignClient(c.s3)
	// Normalize the sellerID to be URL-safe
	normalizedSellerID := strings.ReplaceAll(sellerID, " ", "-")

	key := fmt.Sprintf("verification/kyc/%s/kyc-selfie.jpg", normalizedSellerID)
	presignResult, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:  aws.String(bucketName),
		Key:     aws.String(key),
		Expires: aws.Time(time.Now().Add(15 * time.Minute)),
	})

	if err != nil {
		return "", err
	}

	return presignResult.URL, nil
}
