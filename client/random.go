package client

// RandomData holds the full random response from the server, including data needed
// for validation.
type RandomData struct {
	Rnd               uint64 `json:"round,omitempty"`
	Random            []byte `json:"randomness,omitempty"`
	Sig               []byte `json:"signature,omitempty"`
	PreviousSignature []byte `json:"previous_signature,omitempty"`
}

// GetRound provides access to the round associated with this random data.
func (r *RandomData) GetRound() uint64 {
	return r.Rnd
}

// GetSignature provides the signature over this round's randomness
func (r *RandomData) GetSignature() []byte {
	return r.Sig
}

// GetPreviousSignature provides the previous signature provided by the beacon,
// if nil, it's most likely using an unchained scheme.
func (r *RandomData) GetPreviousSignature() []byte {
	return r.PreviousSignature
}

// GetRandomness exports the randomness using the legacy SHA256 derivation path
func (r *RandomData) GetRandomness() []byte {
	return r.Random
}
