package signer

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	ActionType           = "Action"                     // ActionType represents the key for the action type in the request.
	ActionName           = "kafka-cluster:Connect"      // ActionName represents the specific action name for connecting to a Kafka cluster.
	SigningName          = "kafka-cluster"              // SigningName represents the signing name for the Kafka cluster.
	UserAgentKey         = "User-Agent"                 // UserAgentKey represents the key for the User-Agent parameter in the request.
	LibName              = "aws-msk-iam-sasl-signer-go" // LibName represents the name of the library.
	ExpiresQueryKey      = "X-Amz-Expires"              // ExpiresQueryKey represents the key for the expiration time in the query parameters.
	DefaultSessionName   = "MSKSASLDefaultSession"      // DefaultSessionName represents the default session name for assuming a role.
	DefaultExpirySeconds = 900                          // DefaultExpirySeconds represents the default expiration time in seconds.
)

var (
	endpointURLTemplate = "kafka.%s.amazonaws.com" // endpointURLTemplate represents the template for the Kafka endpoint URL
)

// GenerateAuthToken generates base64 encoded signed url as auth token from default credentials.
// Loads the IAM credentials from default credentials provider chain.
func GenerateAuthToken(ctx context.Context, region string) (string, error) {
	credentials, err := loadDefaultCredentials(ctx, region)

	if err != nil {
		return "", fmt.Errorf("failed to load credentials: %w", err)
	}

	return constructAuthToken(ctx, region, credentials)
}

// GenerateAuthTokenFromProfile generates base64 encoded signed url as auth token by loading IAM credentials from an AWS named profile.
func GenerateAuthTokenFromProfile(ctx context.Context, region string, awsProfile string) (string, error) {
	credentials, err := loadCredentialsFromProfile(ctx, region, awsProfile)

	if err != nil {
		return "", fmt.Errorf("failed to load credentials: %w", err)
	}

	return constructAuthToken(ctx, region, credentials)
}

// GenerateAuthTokenFromRole generates base64 encoded signed url as auth token by loading IAM credentials from an aws role Arn
func GenerateAuthTokenFromRole(
	ctx context.Context, region string, roleArn string, stsSessionName string,
) (string, error) {
	if stsSessionName == "" {
		stsSessionName = DefaultSessionName
	}
	credentials, err := loadCredentialsFromRoleArn(ctx, region, roleArn, stsSessionName)

	if err != nil {
		return "", fmt.Errorf("failed to load credentials: %w", err)
	}

	return constructAuthToken(ctx, region, credentials)
}

// GenerateAuthTokenFromCredentialsProvider generates base64 encoded signed url as auth token by loading IAM credentials
// from an aws credentials provider
func GenerateAuthTokenFromCredentialsProvider(
	ctx context.Context, region string, credentialsProvider aws.CredentialsProvider,
) (string, error) {
	credentials, err := loadCredentialsFromCredentialsProvider(ctx, credentialsProvider)

	if err != nil {
		return "", fmt.Errorf("failed to load credentials: %w", err)
	}

	return constructAuthToken(ctx, region, credentials)
}

// Loads credentials from the default credential chain.
func loadDefaultCredentials(ctx context.Context, region string) (*aws.Credentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))

	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return loadCredentialsFromCredentialsProvider(ctx, cfg.Credentials)
}

// Loads credentials from a named aws profile.
func loadCredentialsFromProfile(ctx context.Context, region string, awsProfile string) (*aws.Credentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithSharedConfigProfile(awsProfile),
	)

	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return loadCredentialsFromCredentialsProvider(ctx, cfg.Credentials)
}

// Loads credentials from a named by assuming the passed role.
// This implementation creates a new sts client for every call to get or refresh token. In order to avoid this, please
// use your own credentials provider.
// If you wish to use regional endpoint, please pass your own credentials provider.
func loadCredentialsFromRoleArn(
	ctx context.Context, region string, roleArn string, stsSessionName string,
) (*aws.Credentials, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))

	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)

	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String(stsSessionName),
	}
	assumeRoleOutput, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return nil, fmt.Errorf("unable to assume role, %s: %w", roleArn, err)
	}

	//Create new aws.Credentials instance using the credentials from AssumeRoleOutput.Credentials
	creds := aws.Credentials{
		AccessKeyID:     *assumeRoleOutput.Credentials.AccessKeyId,
		SecretAccessKey: *assumeRoleOutput.Credentials.SecretAccessKey,
		SessionToken:    *assumeRoleOutput.Credentials.SessionToken,
	}

	return &creds, nil
}

// Loads credentials from the credentials provider
func loadCredentialsFromCredentialsProvider(
	ctx context.Context, credentialsProvider aws.CredentialsProvider,
) (*aws.Credentials, error) {
	creds, err := credentialsProvider.Retrieve(ctx)
	return &creds, err
}

// Constructs Auth Token.
func constructAuthToken(ctx context.Context, region string, credentials *aws.Credentials) (string, error) {
	endpointURL := fmt.Sprintf(endpointURLTemplate, region)

	req, err := buildRequest(DefaultExpirySeconds, endpointURL)
	if err != nil {
		return "", fmt.Errorf("failed to build request for signing: %w", err)
	}

	signedURL, err := signRequest(ctx, req, region, credentials)
	if err != nil {
		return "", fmt.Errorf("failed to sign request with aws sig v4: %w", err)
	}

	signedURLWithUserAgent, err := addUserAgent(signedURL)
	if err != nil {
		return "", fmt.Errorf("failed to add user agent to the signed url: %w", err)
	}

	return base64Encode(signedURLWithUserAgent), nil
}

// Build https request with query parameters in order to sign.
func buildRequest(expirySeconds int, endpointURL string) (*http.Request, error) {
	query := url.Values{
		ActionType:      {ActionName},
		ExpiresQueryKey: {strconv.FormatInt(int64(expirySeconds), 10)},
	}

	authURL := url.URL{
		Host:     endpointURL,
		Scheme:   "https",
		Path:     "/",
		RawQuery: query.Encode(),
	}

	return http.NewRequest(http.MethodGet, authURL.String(), nil)
}

// Sign request with aws sig v4.
func signRequest(ctx context.Context, req *http.Request, region string, credentials *aws.Credentials) (string, error) {
	signer := v4.NewSigner()
	signedURL, _, err := signer.PresignHTTP(ctx, *credentials, req,
		calculateSHA256Hash(""),
		SigningName,
		region,
		time.Now().UTC(),
	)

	return signedURL, err
}

// Calculate sha256Hash and hex encode it.
func calculateSHA256Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// Base64 encode with raw url encoding.
func base64Encode(signedURL string) string {
	signedURLBytes := []byte(signedURL)
	return base64.RawURLEncoding.EncodeToString(signedURLBytes)
}

// Add user agent to the signed url
func addUserAgent(signedURL string) (string, error) {
	parsedSignedURL, err := url.Parse(signedURL)

	if err != nil {
		return "", fmt.Errorf("failed to parse signed url: %w", err)
	}

	query := parsedSignedURL.Query()
	userAgent := strings.Join([]string{LibName, version, runtime.Version()}, "/")
	query.Set(UserAgentKey, userAgent)
	parsedSignedURL.RawQuery = query.Encode()

	return parsedSignedURL.String(), nil
}
