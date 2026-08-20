package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/monkescience/yeet/internal/config"
	"github.com/monkescience/yeet/internal/httptrace"
)

const httpMethodQuery = "QUERY"

type requestMethodError struct {
	method string
	err    error
}

func (e *requestMethodError) Error() string {
	return e.err.Error()
}

func (e *requestMethodError) Unwrap() error {
	return e.err
}

type requestMethodTransport struct {
	next http.RoundTripper
}

func (t requestMethodTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.next.RoundTrip(request)
	if err != nil {
		return response, &requestMethodError{method: request.Method, err: err}
	}

	return response, nil
}

func newTracedRetryableClient(forge string) *retryablehttp.Client {
	return newTracedRetryableClientWithConfig(forge, config.Default().Network)
}

func newTracedRetryableClientWithConfig(
	forge string,
	network config.NetworkConfig,
) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = network.Retry.MaxAttempts - 1
	client.RetryWaitMin = network.Retry.MinBackoff
	client.RetryWaitMax = network.Retry.MaxBackoff
	client.CheckRetry = methodAwareRetryPolicy
	client.Logger = nil
	client.HTTPClient.Timeout = network.RequestTimeout

	trace := httptrace.New(forge)
	client.RequestLogHook = trace.RequestHook
	client.HTTPClient.Transport = requestMethodTransport{next: trace.Interceptor(client.HTTPClient.Transport)}

	return client
}

func methodAwareRetryPolicy(ctx context.Context, response *http.Response, err error) (bool, error) {
	if ctx.Err() != nil {
		return false, fmt.Errorf("retry request: %w", ctx.Err())
	}

	method := requestMethod(response, err)
	if err != nil {
		return isIdempotentHTTPMethod(method) || isPreWriteTransportError(err), nil
	}

	if response == nil {
		return false, nil
	}

	if response.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}

	retryableServerFailure := response.StatusCode == 0 ||
		(response.StatusCode >= http.StatusInternalServerError && response.StatusCode != http.StatusNotImplemented)

	return retryableServerFailure && isIdempotentHTTPMethod(method), nil
}

func requestMethod(response *http.Response, err error) string {
	if response != nil && response.Request != nil {
		return response.Request.Method
	}

	if methodErr, ok := errors.AsType[*requestMethodError](err); ok {
		return methodErr.method
	}

	return ""
}

func isIdempotentHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodTrace,
		http.MethodPut,
		http.MethodDelete,
		httpMethodQuery:
		return true
	default:
		return false
	}
}

func isPreWriteTransportError(err error) bool {
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
		return !dnsErr.IsNotFound
	}

	var operationErr *net.OpError
	if errors.As(err, &operationErr) && strings.EqualFold(operationErr.Op, "dial") {
		return true
	}

	return strings.Contains(err.Error(), "net/http: TLS handshake timeout")
}
