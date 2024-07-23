/*
Package lp2p provides a drand client implementation that retrieves
randomness by subscribing to a libp2p pubsub topic.

WARNING: this client can only be used to "Watch" for new randomness rounds and
"Get" randomness rounds it has previously seen that are still in the cache.

If you need to "Get" arbitrary rounds from the chain then you must combine this client with the http client.
You can Wrap multiple client together.

The agnostic client builder must receive "WithChainInfo()" in order for it to
validate randomness rounds it receives, or "WithChainHash()" and be combined
with the HTTP client implementations so that chain information can be fetched from them.

It is particularly important that rounds are verified since they can be delivered by any peer in the network.
*/
package lp2p
