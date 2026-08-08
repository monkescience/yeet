package provider

import (
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/monkescience/yeet/internal/httptrace"
)

const (
	httpClientTimeout = 30 * time.Second
	httpRetryMax      = 3
	httpRetryWaitMin  = 1 * time.Second
	httpRetryWaitMax  = 10 * time.Second
)

// newTracedRetryableClient builds the one transport policy every forge client
// is constructed with: bounded retries, a request timeout, and sanitized HTTP
// tracing tagged with the forge it was built for.
func newTracedRetryableClient(forge string) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = httpRetryMax
	client.RetryWaitMin = httpRetryWaitMin
	client.RetryWaitMax = httpRetryWaitMax
	client.Logger = nil
	client.HTTPClient.Timeout = httpClientTimeout

	trace := httptrace.New(forge)
	client.RequestLogHook = trace.RequestHook
	client.HTTPClient.Transport = trace.Interceptor(client.HTTPClient.Transport)

	return client
}
