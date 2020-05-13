package bc

import (
	"bytes"
	"crypto/ed25519"
	"encoding/gob"
	"errors"
)

const (
	MinFee = uint64(1)
)

type Transaction interface {
	//todo не все методы в интерфейсе нужны. если будут в будущем использоваться, тогда и добавишь. интерфейсы стоит держать настолько небольшыми, насколько это возможно.
	//SignTransaction(key ed25519.PrivateKey) error
	//Bytes() ([]byte, error)
	Hash() (string, error)
	Verify() error
	Execute(string, *State) error
	GetSenderAddress() string
	GetPrice() uint64
	GetUsers() []string
}

type Transfer struct {
	From   string
	To     string
	Amount uint64
	Fee    uint64
	PubKey ed25519.PublicKey

	Signature []byte `json:"-"`
}

type DeleteMe struct {
	From   string
	Fee    uint64
	PubKey ed25519.PublicKey

	Signature []byte
}

func (t *Transfer) SignTransaction(key ed25519.PrivateKey) error {
	b, err := t.Bytes()
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

func (t *Transfer) Bytes() ([]byte, error) {
	b := bytes.NewBuffer(nil)
	err := gob.NewEncoder(b).Encode(t)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

func (t Transfer) Verify() error {
	if t.Fee < MinFee {
		return errors.New("fee error")
	}

	tr := t
	tr.Signature = []byte{}

	b, err := tr.Bytes()
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

func (t *Transfer) Execute(address string, state *State) error {
	if _, ok := state.users[t.From]; !ok {
		errors.New("user " + t.From + " not exist")
	}
	if _, ok := state.users[t.To]; !ok {
		errors.New("user " + t.To + " not exist")
	}
	if state.users[t.From] < t.Amount+t.Fee {
		errors.New("user " + t.From + " has lack of balance")
	}

	state.users[t.From] = state.users[t.From] - t.Amount - t.Fee
	state.users[t.To] = state.users[t.To] + t.Amount
	state.users[address] += t.Fee

	return nil
}

func (t *Transfer) GetSenderAddress() string {
	return t.From
}

func (t *Transfer) GetPrice() uint64 {
	return t.Amount + t.Fee
}

func (t *Transfer) GetUsers() []string {
	users := []string{t.From, t.To}

	return users
}
