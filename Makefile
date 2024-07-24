drand-relay-gossip:
	go build -o drand-relay-gossip ./gossip-relay/main.go

client-tool:
	go build -o drand-cli ./main.go
