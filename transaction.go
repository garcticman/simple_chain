package bc

import (
	"bytes"
	"crypto/ed25519"
	"encoding/gob"
)

type Transaction struct {
	From   string
	To     string
	Amount uint64
	Fee    uint64
	PubKey ed25519.PublicKey

	Signature []byte `json:"-"`
}

func (t *Transaction) SignTransaction(key ed25519.PrivateKey) error {
	b, err := t.Bytes()
	if err != nil {
		return err
	}

	t.Signature = ed25519.Sign(key, b)
	return nil
}

func (t Transaction) Hash() (string, error) {
	b, err := Bytes(t)
	if err != nil {
		return "", err
	}
	return Hash(b)
}

func (t Transaction) Bytes() ([]byte, error) {
	b := bytes.NewBuffer(nil)
	err := gob.NewEncoder(b).Encode(t)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
