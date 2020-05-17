package bc

import (
	"crypto/ed25519"
	"errors"
)

const (
	MinFee = uint64(1)
)

type Transaction interface {
	SignTransaction(key ed25519.PrivateKey) error
	Hash() (string, error)
	Verify() error
	Execute(string, *map[string]uint64, *Validators) error
	GetSenderAddress() string
	GetPrice() uint64
	GetUsersAndRemovedValidator() ([]string, ed25519.PublicKey)
	GetNonce() uint64
}

type Transfer struct {
	From   string
	To     string
	Amount uint64
	Fee    uint64
	PubKey ed25519.PublicKey
	Nonce  uint64

	Signature []byte `json:"-"`
}

type DeleteMe struct {
	From   string
	Fee    uint64
	PubKey ed25519.PublicKey
	Nonce  uint64

	Signature []byte
}

func (t *Transfer) SignTransaction(key ed25519.PrivateKey) error {
	b, err := Bytes(t)
	if err != nil {
		return err
	}

	t.Signature = ed25519.Sign(key, b)
	return nil
}

func (t *Transfer) Hash() (string, error) {
	b, err := Bytes(t)
	if err != nil {
		return "", err
	}
	return Hash(b), nil
}

func (t *Transfer) Verify() error {
	if t.Fee < MinFee {
		return errors.New("fee error")
	}

	tr := *t
	tr.Signature = nil

	b, err := Bytes(t)
	if err != nil {
		return err
	}
	if address, err := PubKeyToAddress(t.PubKey); address != t.From || err != nil {
		return errors.New("wrong address")
	}
	if !ed25519.Verify(t.PubKey, b, t.Signature) {
		return errors.New("wrong signature")
	}

	return nil
}

func (t *Transfer) Execute(address string, users *map[string]uint64, _ *Validators) error {
	if _, ok := (*users)[t.From]; !ok {
		errors.New("user " + t.From + " not exist")
	}
	if _, ok := (*users)[t.To]; !ok {
		errors.New("user " + t.To + " not exist")
	}
	if (*users)[t.From] < t.Amount+t.Fee {
		errors.New("user " + t.From + " has lack of balance")
	}

	(*users)[t.From] = (*users)[t.From] - t.Amount - t.Fee
	(*users)[t.To] = (*users)[t.To] + t.Amount
	(*users)[address] += t.Fee

	return nil
}

func (t *Transfer) GetSenderAddress() string {
	return t.From
}

func (t *Transfer) GetPrice() uint64 {
	return t.Amount + t.Fee
}

func (t *Transfer) GetNonce() uint64 {
	return t.Nonce
}

func (t *Transfer) GetUsersAndRemovedValidator() ([]string, ed25519.PublicKey) {
	users := []string{t.From, t.To}

	return users, nil
}

func (d *DeleteMe) SignTransaction(key ed25519.PrivateKey) error {
	b, err := Bytes(d)
	if err != nil {
		return err
	}

	d.Signature = ed25519.Sign(key, b)
	return nil
}

func (d *DeleteMe) Hash() (string, error) {
	b, err := Bytes(d)
	if err != nil {
		return "", err
	}
	return Hash(b), nil
}

func (d *DeleteMe) Verify() error {
	if d.Fee < MinFee {
		return errors.New("fee error")
	}

	tr := *d
	tr.Signature = nil

	b, err := Bytes(tr)
	if err != nil {
		return err
	}
	if address, err := PubKeyToAddress(d.PubKey); address != d.From || err != nil {
		return errors.New("wrong address")
	}
	if !ed25519.Verify(d.PubKey, b, d.Signature) {
		return errors.New("wrong signature")
	}

	return nil
}

func (d *DeleteMe) Execute(address string, users *map[string]uint64, validators *Validators) error {
	if _, ok := (*users)[d.From]; !ok {
		errors.New("user " + d.From + " not exist")
	}
	if (*users)[d.From] < d.Fee {
		errors.New("user " + d.From + " has lack of balance")
	}
	(*users)[d.From] = (*users)[d.From] - d.Fee
	(*users)[address] += d.Fee

	err := validators.RemoveValidator(d.PubKey)
	if err != nil {
		return err
	}

	return nil
}

func (d *DeleteMe) GetSenderAddress() string {
	return d.From
}

func (d *DeleteMe) GetNonce() uint64 {
	return d.Nonce
}

func (d *DeleteMe) GetPrice() uint64 {
	return d.Fee
}

func (d *DeleteMe) GetUsersAndRemovedValidator() ([]string, ed25519.PublicKey) {
	users := []string{d.From}

	return users, d.PubKey
}
