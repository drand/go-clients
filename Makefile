.PHONY: drand-relay-gossip client-tool drand-client build clean

build: drand-relay-gossip client-tool

clean:
	rm -f ./drand-relay-gossip ./drand-cli ./drand-client

drand-relay-gossip:
	go build -o drand-relay-gossip ./gossip-relay/main.go

client-tool:
	go build -o drand-cli ./main.go

# same tool as client-tool, under the binary name our deployment scripts expect
drand-client:
	go build -o drand-client ./main.go
