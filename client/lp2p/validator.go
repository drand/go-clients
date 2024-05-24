package lp2p

import (
	"bytes"
	"context"
	"github.com/drand/drand-cli/client"
	commonutils "github.com/drand/drand/v2/common"
	"time"

	clock "github.com/jonboulle/clockwork"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"google.golang.org/protobuf/proto"

	chain2 "github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/protobuf/drand"
)

func randomnessValidator(info *chain2.Info, cache client.Cache, c *Client, clk clock.Clock) pubsub.ValidatorEx {
	return func(ctx context.Context, p peer.ID, m *pubsub.Message) pubsub.ValidationResult {
		rand := &drand.PublicRandResponse{}
		err := proto.Unmarshal(m.Data, rand)
		if err != nil {
			c.log.Warnw("", "gossip validator", "Not validating received randomness due to proto.Unmarshal error", "err", err)
			return pubsub.ValidationReject
		}

		c.log.Debugw("", "gossip validator", "Received new round", "round", rand.GetRound())

		if info == nil {
			c.log.Warnw("", "gossip validator", "Not validating received randomness due to lack of trust root.")
			return pubsub.ValidationAccept
		}

		// Unwilling to relay beacons in the future.
		timeNow := clk.Now()
		timeOfRound := commonutils.TimeOfRound(info.Period, info.GenesisTime, rand.GetRound())
		if time.Unix(timeOfRound, 0).After(timeNow) {
			c.log.Warnw("",
				"gossip validator", "Not validating received randomness due to time of round",
				"err", err,
				"timeOfRound", timeOfRound,
				"time.Now", timeNow.Unix(),
				"info.Period", info.Period,
				"info.Genesis", info.GenesisTime,
				"round", rand.GetRound(),
			)
			return pubsub.ValidationReject
		}

		if cache != nil {
			if current := cache.TryGet(rand.GetRound()); current != nil {
				currentFull, ok := current.(*client.RandomData)
				if !ok {
					// Note: this shouldn't happen in practice, but if we have a
					// degraded cache entry we can't validate the full byte
					// sequence.
					if bytes.Equal(rand.GetSignature(), current.GetSignature()) {
						c.log.Warnw("", "gossip validator", "ignore")
						return pubsub.ValidationIgnore
					}
					c.log.Warnw("", "gossip validator", "reject")
					return pubsub.ValidationReject
				}
				if current.GetRound() == rand.GetRound() &&
					bytes.Equal(current.GetRandomness(), rand.GetRandomness()) &&
					bytes.Equal(current.GetSignature(), rand.GetSignature()) &&
					bytes.Equal(currentFull.PreviousSignature, rand.GetPreviousSignature()) {
					c.log.Warnw("", "gossip validator", "ignore")
					return pubsub.ValidationIgnore
				}
				c.log.Warnw("", "gossip validator", "reject")
				return pubsub.ValidationReject
			}
		}
		scheme, err := crypto.SchemeFromName(info.Scheme)
		if err != nil {
			c.log.Warnw("", "gossip validator", "reject", "err", err)
			return pubsub.ValidationReject
		}

		err = scheme.VerifyBeacon(rand, info.PublicKey)
		if err != nil {
			c.log.Warnw("", "gossip validator", "reject", "err", err)
			return pubsub.ValidationReject
		}
		return pubsub.ValidationAccept
	}
}
