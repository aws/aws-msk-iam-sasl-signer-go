package signer

import (
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// SignerOptions Input options for Signer library.
type SignerOptions struct {
	Region          *string          // Region specifies the AWS region to be used for signing requests.
	AwsProfile      *string          // AwsProfile specifies the AWS profile to be used for loading credentials.
	RoleARN         *string          // RoleARN specifies the AWS Role ARN for assuming a role.
	STSSessionName  *string          // STSSessionName specifies the session name for the assumed role.
	STSRegion       *string          // STSRegion specifies the AWS region to be used for the STS client.
	AwsMaxRetries   int              // AwsMaxRetries specifies the maximum number of retries for AWS SDK requests.
	AwsMaxBackOffMs int              // AwsMaxBackOffMs specifies the maximum backoff duration in milliseconds for AWS SDK requests.
	AWSCredentials  *aws.Credentials // AWSCredentials specifies the credentials to be used to generate signed url.
}

func (so *SignerOptions) Validate() error {
	if so == nil || so.Region == nil {
		return fmt.Errorf("region must be provided")
	}

	credentialsArgumentsCount := 0

	if so.AwsProfile != nil {
		credentialsArgumentsCount++
	}
	if so.RoleARN != nil {
		credentialsArgumentsCount++
	}
	if so.AWSCredentials != nil {
		credentialsArgumentsCount++
	}

	if credentialsArgumentsCount > 1 {
		return fmt.Errorf("please provide only one of AWS profile, Role ARN and AWS Credentials")
	}

	return nil
}
