.PHONY: drand-relay-gossip client-tool build clean

build: drand-relay-gossip client-tool

clean:
	rm -f ./drand-relay-gossip ./drand-cli

drand-relay-gossip:
	go build -o drand-relay-gossip ./gossip-relay/main.go

client-tool:
	go build -o drand-cli ./main.go
