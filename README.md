# AWS MSK IAM SASL Signer for Go

[![Go Build status](https://github.com/aws/aws-msk-iam-sasl-signer-go/actions/workflows/go.yml/badge.svg?branch=main)](https://github.com/aws/aws-msk-iam-sasl-signer-go/actions/workflows/go.yml) [![Apache V2 License](https://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/aws/aws-msk-iam-sasl-signer-go/blob/main/LICENSE.txt)

`aws-msk-iam-sasl-signer-go` is the AWS MSK IAM SASL Signer for Go programming language.

The AWS MSK IAM SASL Signer for Go requires a minimum version of `Go 1.17`.

Check out the [release notes](https://github.com/aws/aws-msk-iam-sasl-signer-go/blob/main/CHANGELOG.md) for information about the latest bug
fixes, updates, and features added to the library.

Jump To:
* [Getting Started](#getting-started)
* [Getting Help](#getting-help)
* [Contributing](#feedback-and-contributing)
* [More Resources](#resources)


## Getting started
To get started working with the AWS MSK IAM SASL Signer for Go with your Kafka client library please follow below code sample -

###### Add Dependencies
```sh
$ go get github.com/aws/aws-msk-iam-sasl-signer-go
```

###### Write Code

For example, you can use the signer library to generate IAM default credentials based OAUTH token with [IBM sarama library](https://github.com/IBM/sarama) as below -

```go
package main

import (
  "context"
  "crypto/tls"
  "log"
  "os"
  "os/signal"
  "time"
  
  "github.com/aws/aws-msk-iam-sasl-signer-go/signer"
  "github.com/Shopify/sarama"
)

var (
  kafkaBrokers = []string{"<your_msk_bootstrap_string>"}
  KafkaTopic = "<your topic name>"
  enqueued int
)

type MSKAccessTokenProvider struct {
}

func (m *MSKAccessTokenProvider) Token() (*sarama.AccessToken, error) {
  token, _, err := signer.GenerateAuthToken(context.TODO(), "<region>")
  return &sarama.AccessToken{Token: token}, err}

func main() {
  sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)
  producer, err := setupProducer()
  if err != nil {
    panic(err)
  } else {
    log.Println("Kafka AsyncProducer up and running!")
  }

  // Trap SIGINT to trigger a graceful shutdown.
  signals := make(chan os.Signal, 1)
  signal.Notify(signals, os.Interrupt)

  produceMessages(producer, signals)

  log.Printf("Kafka AsyncProducer finished with %d messages produced.", enqueued)
}

// setupProducer will create a AsyncProducer and returns it
func setupProducer() (sarama.AsyncProducer, error){
  // Set the SASL/OAUTHBEARER configuration
  config := sarama.NewConfig()
  config.Net.SASL.Enable = true
  config.Net.SASL.Mechanism = sarama.SASLTypeOAuth
  config.Net.SASL.TokenProvider = &MSKAccessTokenProvider{}

  tlsConfig := tls.Config{}
  config.Net.TLS.Enable = true
  config.Net.TLS.Config = &tlsConfig
  return sarama.NewAsyncProducer(kafkaBrokers, config)
}

// produceMessages will send 'testing 123' to KafkaTopic each second, until receive a os signal to stop e.g. control + c
// by the user in terminal
func produceMessages(producer sarama.AsyncProducer, signals chan os.Signal) {
  for {
    time.Sleep(time.Second)
    message := &sarama.ProducerMessage{Topic: KafkaTopic, Value: sarama.StringEncoder("testing 123")}
    select {
    case producer.Input() <- message:
      enqueued++
      log.Println("New Message produced")
    case <-signals:
      producer.AsyncClose() // Trigger a shutdown of the producer.
      return
    }
  }
}
```

Consumer -

```go
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/awslabs/aws-msk-iam-sasl-signer-go/signer"
	"github.com/Shopify/sarama"
)

var (
  kafkaBrokers = []string{"<your_msk_bootstrap_string>"}
  KafkaTopic = "<your topic name>"
)

type MSKAccessTokenProvider struct {
}

func (m *MSKAccessTokenProvider) Token() (*sarama.AccessToken, error) {
	token, _, err := signer.GenerateAuthToken(context.TODO(), "<region>")
	return &sarama.AccessToken{Token: token}, err
}

func main() {
	sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)
	consumer, err := setUpConsumer()
	if err != nil {
		panic(err)
	} else {
		log.Println("Kafka Consumer is up and running!")
	}

	defer func() {
		if err := consumer.Close(); err != nil {
			log.Printf("Error closing consumer: %w", err)
		}
	}()

	consumeMessages(consumer)
}

func setUpConsumer() (sarama.Consumer, error) {
	// Set the SASL/OAUTHBEARER configuration
	config := sarama.NewConfig()
	config.Net.SASL.Enable = true
	config.Net.SASL.Mechanism = sarama.SASLTypeOAuth
	config.Net.SASL.TokenProvider = &MSKAccessTokenProvider{}

	tlsConfig := tls.Config{}
	config.Net.TLS.Enable = true
	config.Net.TLS.Config = &tlsConfig
	return sarama.NewConsumer(kafkaBrokers, config)
}

func consumeMessages(consumer sarama.Consumer) {
	partitions, err := consumer.Partitions(KafkaTopic)
	if err != nil {
		log.Fatalf("Failed to retrieve partitions for topic %s: %v", KafkaTopic, err)
	}

	consumers := make(chan *sarama.ConsumerMessage)
	errors := make(chan *sarama.ConsumerError)

	// Create a partition consumer and goroutine for each partition
	for _, partition := range partitions {
		partitionConsumer, err := consumer.ConsumePartition(KafkaTopic, partition, sarama.OffsetNewest)
		if err != nil {
			log.Fatalf("Failed to create partition consumer for topic %s, partition %d: %v", KafkaTopic, partition, err)
		}

		go func(KafkaTopic string, partitionConsumer sarama.PartitionConsumer) {
			for {
				select {
				case consumerError := <-partitionConsumer.Errors():
					errors <- consumerError

				case msg := <-partitionConsumer.Messages():
					consumers <- msg
				}
			}
		}(KafkaTopic, partitionConsumer)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	msgCount := 0

	doneCh := make(chan struct{})
	go func() {
		for {
			select {
			case msg := <-consumers:
				msgCount++
				fmt.Println("Received message : ", string(msg.Key), string(msg.Value))
			case consumerError := <-errors:
				msgCount++
				fmt.Println("Received consumerError ", string(consumerError.Topic), string(consumerError.Partition), consumerError.Err)
				doneCh <- struct{}{}
			case <-signals:
				fmt.Println("Interrupt is detected")
				doneCh <- struct{}{}
			}
		}
	}()

	<-doneCh
	fmt.Println("Processed", msgCount, "messages")
}


```

* To use IAM credentials from a named profile, update the Token() function: 
```go
func (t *MSKAccessTokenProvider) Token() (*sarama.AccessToken, error) {
	token, _, err := signer.GenerateAuthTokenFromProfile(context.TODO(), "<region>", "<namedProfile>")
	return &sarama.AccessToken{Token: token}, err
}
```

* To use IAM credentials by assuming a IAM Role using sts, update the Token() function:

```go
func (t *MSKAccessTokenProvider) Token() (*sarama.AccessToken, error) {
        token, _, err := signer.GenerateAuthTokenFromRole(context.TODO(), "<region>", "<my-role-arn>", "my-sts-session-name")
        return &sarama.AccessToken{Token: token}, err
}
```
* To use IAM credentials from a credentials provider, update the Token() function:
```go
func (t *MSKAccessTokenProvider) Token() (*sarama.AccessToken, error) {
        token, _, err := signer.GenerateAuthTokenFromCredentialsProvider(context.TODO(), "<region>", <MyCredentialsProvider>)
        return &sarama.AccessToken{Token: token}, err
}
```


###### Compile and Execute
```sh
$ go build
$ go run .
```

###### Test
```sh
$ cd signer
$ go test
```

## Troubleshooting
### Finding out which identity is being used
You may receive an `Access denied` error and there may be some doubt as to which credential is being exactly used. The credential may be sourced from a role ARN, EC2 instance profile, credential profile etc.
You can set the field `AwsDebugCreds` set to true before getting the token:

```go
    signer.AwsDebugCreds = true
```
the client library will print a debug log of the form:
```
Credentials Identity: {UserId: ABCD:test124, Account: 1234567890, Arn: arn:aws:sts::1234567890:assumed-role/abc/test124}
```

The log line provides the IAM Account, IAM user id and the ARN of the IAM Principal corresponding to the credential being used.

Please note that the log level should also be set to DEBUG for this information to be logged. It is not recommended to run with AwsDebugCreds=true since it makes an additional remote call.

## Getting Help

Please use these community resources for getting help. We use the GitHub issues
for tracking bugs and feature requests.

* Ask us a [question](https://github.com/aws/aws-msk-iam-sasl-signer-go/discussions/new?category=q-a) or open a [discussion](https://github.com/aws/aws-msk-iam-sasl-signer-go/discussions/new?category=general).
* If you think you may have found a bug, please open an [issue](https://github.com/aws/aws-msk-iam-sasl-signer-go/issues/new/choose).
* Open a support ticket with [AWS Support](http://docs.aws.amazon.com/awssupport/latest/user/getting-started.html).

This repository provides a pluggable library with any Go Kafka client for SASL/OAUTHBEARER mechanism. For more information about SASL/OAUTHBEARER mechanism please go to [KIP 255](https://cwiki.apache.org/confluence/pages/viewpage.action?pageId=75968876).

### Opening Issues

If you encounter a bug with the AWS MSK IAM SASL Signer for Go we would like to hear about it.
Search the [existing issues][Issues] and see
if others are also experiencing the same issue before opening a new issue. Please
include the version of AWS MSK IAM SASL Signer for Go, Go language, and OS you’re using. Please
also include reproduction case when appropriate.

The GitHub issues are intended for bug reports and feature requests. For help
and questions with using AWS MSK IAM SASL Signer for Go, please make use of the resources listed
in the [Getting Help](#getting-help) section.
Keeping the list of open issues lean will help us respond in a timely manner.

## Feedback and contributing

The AWS MSK IAM SASL Signer for Go will use GitHub [Issues] to track feature requests and issues with the library. In addition, we'll use GitHub [Projects] to track large tasks spanning multiple pull requests, such as refactoring the library's internal request lifecycle. You can provide feedback to us in several ways.

**GitHub issues**. To provide feedback or report bugs, file GitHub [Issues] on the library. This is the preferred mechanism to give feedback so that other users can engage in the conversation, +1 issues, etc. Issues you open will be evaluated, and included in our roadmap for the GA launch.

**Contributing**. You can open pull requests for fixes or additions to the AWS MSK IAM SASL Signer for Go. All pull requests must be submitted under the Apache 2.0 license and will be reviewed by a team member before being merged in. Accompanying unit tests, where possible, are appreciated.

## Resources

[Developer Guide](https://aws.github.io/aws-msk-iam-sasl-signer-go/docs/) - Use this document to learn how to get started and
use the AWS MSK IAM SASL Signer for Go.

[Service Documentation](https://docs.aws.amazon.com/msk/latest/developerguide/getting-started.html) - Use this
documentation to learn how to interface with AWS MSK.

[Issues] - Report issues, submit pull requests, and get involved
  (see [Apache 2.0 License][license])

[Dep]: https://github.com/golang/dep
[Issues]: https://github.com/aws/aws-msk-iam-sasl-signer-go/issues
[Projects]: https://github.com/aws/aws-msk-iam-sasl-signer-go/projects
[CHANGELOG]: https://github.com/aws/aws-msk-iam-sasl-signer-go/blob/main/CHANGELOG.md
[design]: https://github.com/aws/aws-msk-iam-sasl-signer-go/blob/main/DESIGN.md
[license]: http://aws.amazon.com/apache2.0/
