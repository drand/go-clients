/*
Package grpc provides a drand client implementation that uses drand's gRPC API.

The client connects to a drand gRPC endpoint to fetch randomness. The gRPC
client has some advantages over the HTTP client - it is more compact
on-the-wire and supports streaming and authentication.

A path to a file that holds TLS credentials for the drand server is required
to validate server connections. Alternatively set the final parameter to
`true` to enable _insecure_ connections (not recommended).
*/
package grpc
