package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

// S3Options configures the S3-compatible driver.
type S3Options struct {
	Endpoint       string // e.g. "http://rustfs:9000"; empty means AWS's own endpoint
	Region         string // required by the SDK; most self-hosted backends ignore the value
	Bucket         string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool          // non-AWS endpoints almost always need this
	PresignTTL     time.Duration // <= 0 disables presigned URLs (URL returns "")
}

// S3Driver stores objects in any S3-compatible service: RustFS (recommended),
// Ceph RGW, AWS S3, Aliyun OSS, ...
//
// Only the plain S3 subset is used — PutObject / GetObject / HeadObject /
// DeleteObject / CopyObject / HeadBucket — so the deployment never becomes a
// one-vendor lock-in.
//
// MinIO is deliberately NOT a target: it has been archived and moved to an
// AGPL-only licence. RustFS (Apache-2.0) is the recommended replacement and is
// also the upstream migration path for existing MinIO deployments.
type S3Driver struct {
	client     *s3.Client
	presigner  *s3.PresignClient
	uploader   *transfermanager.Client
	bucket     string
	presignTTL time.Duration
}

// NewS3Driver builds the driver and verifies the bucket is reachable.
//
// The bucket is never created automatically: a typo in S3_ENDPOINT would
// otherwise silently produce an empty bucket and "lose" every upload. Failing
// at startup is the cheaper mistake.
func NewS3Driver(ctx context.Context, opts S3Options) (*S3Driver, error) {
	if opts.Bucket == "" {
		return nil, errors.New("S3_BUCKET is required when STORAGE_DRIVER=s3")
	}
	if opts.AccessKey == "" || opts.SecretKey == "" {
		return nil, errors.New("S3_ACCESS_KEY and S3_SECRET_KEY are required when STORAGE_DRIVER=s3")
	}
	region := opts.Region
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(opts.AccessKey, opts.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if opts.Endpoint != "" {
			o.BaseEndpoint = aws.String(opts.Endpoint)
		}
		o.UsePathStyle = opts.ForcePathStyle
	})

	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(opts.Bucket)}); err != nil {
		return nil, fmt.Errorf("bucket %q not reachable — create it first (e.g. `mc mb`): %w", opts.Bucket, err)
	}

	return &S3Driver{
		client:     client,
		presigner:  s3.NewPresignClient(client),
		uploader:   transfermanager.New(client),
		bucket:     opts.Bucket,
		presignTTL: opts.PresignTTL,
	}, nil
}

// StoreHash streams the content to a temporary object while hashing it, then
// moves it to its content-addressed key.
//
// The two-step dance is unavoidable: the final key *is* the digest, so it can
// only be known after the last byte has been read. CopyObject is a server-side
// operation, so the extra hop costs no client bandwidth.
func (d *S3Driver) StoreHash(reader io.Reader) (hash, relPath string, err error) {
	ctx := context.Background()
	tmpKey := ".tmp-upload-" + uuid.NewString()

	h := sha256.New()
	if _, err = d.uploader.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(tmpKey),
		Body:   io.TeeReader(reader, h),
	}); err != nil {
		return "", "", fmt.Errorf("upload temp object: %w", err)
	}

	hash = hex.EncodeToString(h.Sum(nil))
	relPath = ObjectKey(hash)

	exists, err := d.Exists(relPath)
	if err != nil {
		d.removeQuietly(tmpKey)
		return "", "", err
	}
	if exists { // identical content already stored — dedup
		d.removeQuietly(tmpKey)
		return hash, relPath, nil
	}
	if err = d.copyObject(tmpKey, relPath); err != nil {
		d.removeQuietly(tmpKey)
		return "", "", err
	}
	d.removeQuietly(tmpKey)
	return hash, relPath, nil
}

// Store saves an object at an explicit key.
// Deprecated: prefer StoreHash for content-addressed storage.
func (d *S3Driver) Store(key string, reader io.Reader) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	_, err := d.uploader.UploadObject(context.Background(), &transfermanager.UploadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	return err
}

func (d *S3Driver) Retrieve(key string) (io.ReadCloser, error) {
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	out, err := d.client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	return out.Body, nil
}

func (d *S3Driver) Delete(key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	_, err := d.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (d *S3Driver) Exists(key string) (bool, error) {
	if err := ValidateKey(key); err != nil {
		return false, err
	}
	_, err := d.client.HeadObject(context.Background(), &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	var respErr *awshttp.ResponseError
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("head object %s: %w", key, err)
}

// URL returns a presigned GET URL, or "" when presigning is disabled or the key
// is invalid.
//
// Presigned URLs let browsers fetch large objects straight from the object
// store instead of streaming through the app server, at the cost of bypassing
// application-level audit — which is why it is opt-in.
func (d *S3Driver) URL(key string) string {
	if d.presignTTL <= 0 {
		return ""
	}
	if err := ValidateKey(key); err != nil {
		return ""
	}
	out, err := d.presigner.PresignGetObject(context.Background(),
		&s3.GetObjectInput{Bucket: aws.String(d.bucket), Key: aws.String(key)},
		func(o *s3.PresignOptions) { o.Expires = d.presignTTL },
	)
	if err != nil {
		return ""
	}
	return out.URL
}

// copyObject is server-side: the client never re-uploads the bytes.
//
// The source is passed unescaped on purpose — object keys are restricted to
// hex digits, '/', '-' and '.', so no character needs URL encoding.
func (d *S3Driver) copyObject(srcKey, dstKey string) error {
	_, err := d.client.CopyObject(context.Background(), &s3.CopyObjectInput{
		Bucket:     aws.String(d.bucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(d.bucket + "/" + srcKey),
	})
	if err != nil {
		return fmt.Errorf("copy object %s -> %s: %w", srcKey, dstKey, err)
	}
	return nil
}

// removeQuietly deletes a temporary object; failures are not actionable for the
// caller, whose upload already succeeded or already failed for another reason.
func (d *S3Driver) removeQuietly(key string) {
	_, _ = d.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
}
