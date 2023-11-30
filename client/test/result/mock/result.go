package mock

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/crypto"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/sign/tbls"
	"github.com/drand/kyber/util/random"
)

// NewMockResult creates a mock result for testing.
func NewMockResult(round uint64) Result {
	sig := make([]byte, 8)
	binary.LittleEndian.PutUint64(sig, round)
	return Result{
		Rnd:  round,
		Sig:  sig,
		Rand: crypto.RandomnessFromSignature(sig),
	}
}

// Result is a mock result that can be used for testing.
type Result struct {
	Rnd  uint64
	Rand []byte
	Sig  []byte
	PSig []byte
}

// GetRandomness is a hash of the signature.
func (r *Result) GetRandomness() []byte {
	return r.Rand
}

// GetSignature is the signature of the randomness for this round.
func (r *Result) GetSignature() []byte {
	return r.Sig
}

// GetPreviousSignature is the signature of the previous round.
func (r *Result) GetPreviousSignature() []byte {
	return r.PSig
}

// GetRound is the round number for this random data.
func (r *Result) GetRound() uint64 {
	return r.Rnd
}

// AssertValid checks that this result is valid.
func (r *Result) AssertValid(t *testing.T) {
	t.Helper()
	sigTarget := make([]byte, 8)
	binary.LittleEndian.PutUint64(sigTarget, r.Rnd)
	if !bytes.Equal(r.Sig, sigTarget) {
		t.Fatalf("expected sig: %x, got %x", sigTarget, r.Sig)
	}
	randTarget := crypto.RandomnessFromSignature(sigTarget)
	if !bytes.Equal(r.Rand, randTarget) {
		t.Fatalf("expected rand: %x, got %x", randTarget, r.Rand)
	}
}

func sha256Hash(prev []byte, round int) []byte {
	h := sha256.New()
	if prev != nil {
		_, _ = h.Write(prev)
	}
	if round > 0 {
		_ = binary.Write(h, binary.BigEndian, uint64(round))
	}
	return h.Sum(nil)
}

func roundToBytes(r int) []byte {
	var buff bytes.Buffer
	binary.Write(&buff, binary.BigEndian, uint64(r))
	return buff.Bytes()
}

// VerifiableResults creates a set of results that will pass a `chain.Verify` check.
func VerifiableResults(count int, sch *crypto.Scheme) (*chain.Info, []Result) {
	secret := sch.KeyGroup.Scalar().Pick(random.New())
	public := sch.KeyGroup.Point().Mul(secret, nil)
	previous := make([]byte, 32)
	if _, err := rand.Reader.Read(previous); err != nil {
		panic(err)
	}

	out := make([]Result, count)
	for i := range out {
		var msg []byte
		if sch.Name == crypto.DefaultSchemeID {
			// we're in chained mode
			msg = sha256Hash(previous[:], i+1)
		} else {
			// we are in unchained mode
			msg = sha256Hash(nil, i+1)
		}

		sshare := share.PriShare{I: 0, V: secret}
		tsig, err := sch.ThresholdScheme.Sign(&sshare, msg)
		if err != nil {
			panic(err)
		}
		tshare := tbls.SigShare(tsig)
		sig := tshare.Value()

		// chained mode
		if sch.Name == crypto.DefaultSchemeID {
			previous = make([]byte, len(sig))
			copy(previous[:], sig)
		} else {
			previous = nil
		}

		out[i] = Result{
			Sig:  sig,
			PSig: previous,
			Rnd:  uint64(i + 1),
			Rand: crypto.RandomnessFromSignature(sig),
		}

	}
	info := chain.Info{
		PublicKey:   public,
		Period:      time.Second,
		GenesisTime: time.Now().Unix() - int64(count),
		GenesisSeed: out[0].PSig,
		Scheme:      sch.Name,
	}

	return &info, out
}
