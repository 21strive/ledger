package ledger

import (
	"context"
	"fmt"
	"mime"
	"regexp"
	"strings"
	"time"

	"github.com/21strive/ledger/domain"
	"github.com/21strive/ledger/ledgererr"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// normalizeSellerID replaces any special characters (non-alphanumeric) with underscores
// to make sellerID safe for use in S3 keys and URLs
func normalizeSellerID(sellerID string) string {
	// Replace any character that's not a letter, number, or underscore with underscore
	re := regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	return re.ReplaceAllString(sellerID, "_")
}

func (c *LedgerClient) GetPhotoKTPPresignedURL(ctx context.Context, sellerID string, bucketName string, contentType string) (string, error) {
	validatedExt, err := validateContentType(contentType)
	if err != nil {
		return "", ledgererr.ErrInvalidRequest.WithError(err)
	}
	// Normalize the sellerID to be URL-safe
	normalizedSellerID := normalizeSellerID(sellerID)

	key := fmt.Sprintf("verification/ktp/%s/ktp.%s", normalizedSellerID, validatedExt)

	presignClient := s3.NewPresignClient(c.s3)
	presignResult, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(15*time.Minute),
	)

	if err != nil {
		return "", ledgererr.ErrInvalidRequest.WithError(err)
	}

	return presignResult.URL, nil
}

func (c *LedgerClient) GetPhotoKYCSelfiePresignedURL(ctx context.Context, sellerID string, bucketName string, contentType string) (string, error) {
	validatedExt, err := validateContentType(contentType)
	if err != nil {
		return "", ledgererr.ErrInvalidRequest.WithError(err)
	}
	// Normalize the sellerID to be URL-safe
	normalizedSellerID := normalizeSellerID(sellerID)

	key := fmt.Sprintf("verification/kyc/%s/kyc-selfie.%s", normalizedSellerID, validatedExt)

	presignClient := s3.NewPresignClient(c.s3)
	presignResult, err := presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(15*time.Minute))

	if err != nil {
		return "", ledgererr.ErrInvalidRequest.WithError(err)
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

// SubmitVerificationRequest contains all data needed to submit a verification
type SubmitVerificationRequest struct {
	AccountUUID    string
	SellerID       string
	IdentityID     string
	Fullname       string
	BirthDate      time.Time
	Province       string
	City           string
	District       string
	PostalCode     string
	KTPPhotoExt    string // File extension (jpg, png, etc.)
	SelfiePhotoExt string
}

// SubmitVerification validates photo existence in S3/R2 and saves verification to database
func (c *LedgerClient) SubmitVerification(ctx context.Context, bucketName string, req SubmitVerificationRequest) (*domain.Verification, error) {
	// Validate required fields
	if req.AccountUUID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "account_uuid is required", nil)
	}
	if req.SellerID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "seller_id is required", nil)
	}
	if req.IdentityID == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "identity_id is required", nil)
	}
	if req.Fullname == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "fullname is required", nil)
	}
	if req.KTPPhotoExt == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "ktp_photo_ext is required", nil)
	}
	if req.SelfiePhotoExt == "" {
		return nil, ledgererr.NewError(ledgererr.CodeInvalidRequest, "selfie_photo_ext is required", nil)
	}

	// Normalize sellerID for S3 keys
	normalizedSellerID := normalizeSellerID(req.SellerID)

	// Construct S3 keys
	ktpKey := fmt.Sprintf("verification/ktp/%s/ktp.%s", normalizedSellerID, req.KTPPhotoExt)
	selfieKey := fmt.Sprintf("verification/kyc/%s/kyc-selfie.%s", normalizedSellerID, req.SelfiePhotoExt)

	// Check if KTP photo exists in S3/R2
	if err := c.checkS3ObjectExists(ctx, bucketName, ktpKey); err != nil {
		return nil, ledgererr.NewError(
			ledgererr.CodeInvalidRequest,
			fmt.Sprintf("KTP photo not found in storage: %s", ktpKey),
			err,
		)
	}

	// Check if selfie photo exists in S3/R2
	if err := c.checkS3ObjectExists(ctx, bucketName, selfieKey); err != nil {
		return nil, ledgererr.NewError(
			ledgererr.CodeInvalidRequest,
			fmt.Sprintf("selfie photo not found in storage: %s", selfieKey),
			err,
		)
	}

	ktpPhotoURL := ktpKey
	selfiePhotoURL := selfieKey

	// Create verification entity
	verification, err := domain.NewVerification(
		req.AccountUUID,
		req.IdentityID,
		req.Fullname,
		req.BirthDate,
		req.Province,
		req.City,
		req.District,
		req.PostalCode,
		ktpPhotoURL,
		selfiePhotoURL,
	)
	if err != nil {
		return nil, err
	}

	// Save to database
	verificationRepo := c.repoProvider.Verification()
	if err := verificationRepo.Save(ctx, verification); err != nil {
		return nil, ledgererr.NewError(
			ledgererr.CodeDatabaseError,
			"failed to save verification",
			err,
		)
	}

	c.logger.Info("verification submitted successfully",
		"account_uuid", req.AccountUUID,
		"seller_id", req.SellerID,
		"identity_id", req.IdentityID,
		"verification_uuid", verification.UUID,
	)

	return verification, nil
}

// checkS3ObjectExists checks if an object exists in S3/R2 using HeadObject
func (c *LedgerClient) checkS3ObjectExists(ctx context.Context, bucketName, key string) error {
	_, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("object not found: %w", err)
	}
	return nil
}
