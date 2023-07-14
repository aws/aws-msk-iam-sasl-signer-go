package signer

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

var (
	TestRegion   = "us-west-2"
	TestEndpoint = "kafka.us-west-2.amazonaws.com"
	Ctx          = context.TODO()
)

// Provides mocked credentials.
type MockCredentialsProvider struct {
	credentials aws.Credentials
}

func (t MockCredentialsProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return t.credentials, nil
}

func TestCalculatePayloadHashForSigning(t *testing.T) {
	sha256HashForEmptyString := calculateSHA256Hash("")
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", sha256HashForEmptyString)

	sha256HashForTestString := calculateSHA256Hash("test")
	assert.Equal(t, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", sha256HashForTestString)
}

func TestAddUserAgent(t *testing.T) {
	signedURL := "https://kafka.us-west-2.amazonaws.com/?Action=kafka-cluster%3AConnect"
	result, err := addUserAgent(signedURL)

	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(result, fmt.Sprintf("%s&%s=%s", signedURL, UserAgentKey, LibName)))
}

func TestAddUserAgentWithInvalidURL(t *testing.T) {
	signedURL := ":invalidURL:"
	result, err := addUserAgent(signedURL)

	assert.Error(t, err)
	assert.Equal(t, "", result)
}

func TestLoadDefaultCredentials(t *testing.T) {
	mockCreds := aws.Credentials{
		AccessKeyID:     "MOCK-ACCESS-KEY",
		SecretAccessKey: "MOCK-SECRET-KEY",
	}

	os.Setenv("AWS_ACCESS_KEY_ID", mockCreds.AccessKeyID)
	os.Setenv("AWS_SECRET_ACCESS_KEY", mockCreds.SecretAccessKey)

	creds, err := loadDefaultCredentials(Ctx, TestRegion)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
	assert.Equal(t, mockCreds.AccessKeyID, creds.AccessKeyID)
	assert.Equal(t, mockCreds.SecretAccessKey, creds.SecretAccessKey)

	// Clean-up env variables.
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")
}

func TestConstructAuthToken(t *testing.T) {
	mockCreds := aws.Credentials{
		AccessKeyID:     "MOCK-ACCESS-KEY",
		SecretAccessKey: "MOCK-SECRET-KEY",
		SessionToken:    "MOCK-SESSION-TOKEN",
	}

	token, err := constructAuthToken(Ctx, TestRegion, &mockCreds)

	assert.NoError(t, err)
	assert.NotNil(t, token)

	decodedSignedURLBytes, err := base64.RawURLEncoding.DecodeString(token)
	assert.NoError(t, err)

	decodedSignedURL := string(decodedSignedURLBytes)

	parsedURL, err := url.Parse(decodedSignedURL)
	assert.NoError(t, err)
	assert.NotNil(t, parsedURL)
	assert.Equal(t, parsedURL.Scheme, "https")
	assert.Equal(t, parsedURL.Host, TestEndpoint)

	params := parsedURL.Query()

	assert.Equal(t, params.Get("Action"), "kafka-cluster:Connect")
	assert.Equal(t, params.Get("X-Amz-Algorithm"), "AWS4-HMAC-SHA256")
	assert.Equal(t, params.Get("X-Amz-Expires"), "900")
	assert.Equal(t, params.Get("X-Amz-Security-Token"), mockCreds.SessionToken)
	assert.Equal(t, params.Get("X-Amz-SignedHeaders"), "host")
	credential := params.Get("X-Amz-Credential")
	splitCredential := strings.Split(credential, "/")
	assert.Equal(t, splitCredential[0], mockCreds.AccessKeyID)
	assert.Equal(t, splitCredential[2], TestRegion)
	assert.Equal(t, splitCredential[3], "kafka-cluster")
	assert.Equal(t, splitCredential[4], "aws4_request")
	date, err := time.Parse("20060102T150405Z", params.Get("X-Amz-Date"))
	assert.NoError(t, err)
	assert.True(t, date.Before(time.Now().UTC()))
	assert.True(t, strings.HasPrefix(params.Get(UserAgentKey), "aws-msk-iam-sasl-signer-go/"))
}

func TestGenerateAuthToken(t *testing.T) {
	mockCreds := aws.Credentials{
		AccessKeyID:     "TEST-ACCESS-KEY",
		SecretAccessKey: "TEST-SECRET-KEY",
		SessionToken:    "TEST-SESSION-TOKEN",
	}

	os.Setenv("AWS_ACCESS_KEY_ID", mockCreds.AccessKeyID)
	os.Setenv("AWS_SECRET_ACCESS_KEY", mockCreds.SecretAccessKey)
	os.Setenv("AWS_SESSION_TOKEN", mockCreds.SessionToken)

	token, err := GenerateAuthToken(Ctx, TestRegion)

	assert.NoError(t, err)
	assert.NotNil(t, token)

	decodedSignedURLBytes, err := base64.RawURLEncoding.DecodeString(token)
	assert.NoError(t, err)

	decodedSignedURL := string(decodedSignedURLBytes)

	parsedURL, err := url.Parse(decodedSignedURL)
	assert.NoError(t, err)
	assert.NotNil(t, parsedURL)
	assert.Equal(t, parsedURL.Scheme, "https")
	assert.Equal(t, parsedURL.Host, TestEndpoint)

	params := parsedURL.Query()

	assert.Equal(t, params.Get("Action"), "kafka-cluster:Connect")
	assert.Equal(t, params.Get("X-Amz-Algorithm"), "AWS4-HMAC-SHA256")
	assert.Equal(t, params.Get("X-Amz-Expires"), "900")
	assert.Equal(t, params.Get("X-Amz-Security-Token"), mockCreds.SessionToken)
	assert.Equal(t, params.Get("X-Amz-SignedHeaders"), "host")
	credential := params.Get("X-Amz-Credential")
	splitCredential := strings.Split(credential, "/")
	assert.Equal(t, splitCredential[0], mockCreds.AccessKeyID)
	assert.Equal(t, splitCredential[2], TestRegion)
	assert.Equal(t, splitCredential[3], "kafka-cluster")
	assert.Equal(t, splitCredential[4], "aws4_request")
	date, err := time.Parse("20060102T150405Z", params.Get("X-Amz-Date"))
	assert.NoError(t, err)
	assert.True(t, date.Before(time.Now().UTC()))
	assert.True(t, strings.HasPrefix(params.Get(UserAgentKey), "aws-msk-iam-sasl-signer-go/"))

	// Clean-up env variables.
	os.Unsetenv("AWS_ACCESS_KEY_ID")
	os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	os.Unsetenv("AWS_SESSION_TOKEN")
}

func TestGenerateAuthTokenWithCredentialsProvider(t *testing.T) {
	mockCreds := aws.Credentials{
		AccessKeyID:     "TEST-MY-ACCESS-KEY",
		SecretAccessKey: "TEST-MY-SECRET-KEY",
	}

	mockCredentialsProvider := MockCredentialsProvider{credentials: mockCreds}

	token, err := GenerateAuthTokenFromCredentialsProvider(Ctx, TestRegion, mockCredentialsProvider)

	assert.NoError(t, err)
	assert.NotNil(t, token)

	decodedSignedURLBytes, err := base64.RawURLEncoding.DecodeString(token)
	assert.NoError(t, err)

	decodedSignedURL := string(decodedSignedURLBytes)

	parsedURL, err := url.Parse(decodedSignedURL)
	assert.NoError(t, err)
	assert.NotNil(t, parsedURL)
	assert.Equal(t, parsedURL.Scheme, "https")
	assert.Equal(t, parsedURL.Host, TestEndpoint)

	params := parsedURL.Query()

	assert.Equal(t, params.Get("Action"), "kafka-cluster:Connect")
	assert.Equal(t, params.Get("X-Amz-Algorithm"), "AWS4-HMAC-SHA256")
	assert.Equal(t, params.Get("X-Amz-Expires"), "900")
	assert.Equal(t, params.Get("X-Amz-Security-Token"), "")
	assert.Equal(t, params.Get("X-Amz-SignedHeaders"), "host")
	credential := params.Get("X-Amz-Credential")
	splitCredential := strings.Split(credential, "/")
	assert.Equal(t, splitCredential[0], mockCreds.AccessKeyID)
	assert.Equal(t, splitCredential[2], TestRegion)
	assert.Equal(t, splitCredential[3], "kafka-cluster")
	assert.Equal(t, splitCredential[4], "aws4_request")
	date, err := time.Parse("20060102T150405Z", params.Get("X-Amz-Date"))
	assert.NoError(t, err)
	assert.True(t, date.Before(time.Now().UTC()))
	assert.True(t, strings.HasPrefix(params.Get(UserAgentKey), "aws-msk-iam-sasl-signer-go/"))
}

func TestGenerateAuthTokenWithFailingCredentialsProvider(t *testing.T) {
	mockCredentialsProvider := aws.AnonymousCredentials{}

	token, err := GenerateAuthTokenFromCredentialsProvider(Ctx, TestRegion, mockCredentialsProvider)

	assert.Error(t, err)
	assert.NotNil(t, token)
}
