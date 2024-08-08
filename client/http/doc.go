/*
Package http provides a drand client implementation that uses drand's HTTP API.

The HTTP client uses drand's JSON HTTP API
(https://drand.love/developer/http-api/) to fetch randomness. Watching is
implemented by polling the endpoint at the expected round time.

The "ForURLs" helper creates multiple HTTP clients from a list of
URLs. Alternatively you can use the "New" or "NewWithInfo" constructor to
create clients.

Tip: Provide multiple URLs to enable failover and speed optimized URL
selection.
*/
package http
