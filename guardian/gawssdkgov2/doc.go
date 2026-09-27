// Package gawssdkgov2 instruments logical AWS SDK for Go v2 API calls for fault injection.
//
// Interceptor uses the SDK's BeforeExecution hook, which runs once outside the retry loop. It does not change the client's retry policy. An injected error can therefore be hidden if another interceptor or application layer retries the entire logical API call. Disable retries when deterministic fault placement is required.
package gawssdkgov2
