package bc

import (
	"crypto"
	"crypto/ed25519"
	"errors"
)

type Block struct {
	BlockNum      uint64
	Timestamp     int64
	Transactions  []Transaction
	BlockHash     string `json:"-"`
	PrevBlockHash string
	StateHash     string
	Signature     []byte `json:"-"`
	SignatureTwo  []byte
}

func LoadGenesis() (genesis Genesis, err error) {
	tempGenesis := struct {
		ChainConfig *ChainConfig
		//Account -> funds
		Alloc map[string]uint64
		//list of validators public keys
		Validators []ed25519.PublicKey
	}{}

	err = ReadFromJSON("Genesis.json", &tempGenesis)
	if err != nil {
		return Genesis{}, err
	}

	//to have ed25519.PublicKey in crypto.PublicKey interface
	genesis = Genesis{
		ChainConfig: tempGenesis.ChainConfig,
		Alloc:       tempGenesis.Alloc,
		Validators:  make([]crypto.PublicKey, len(tempGenesis.Validators)),
	}

	for i, v := range tempGenesis.Validators {
		genesis.Validators[i] = v
	}

	return
}

func (bl *Block) SignBlock(key ed25519.PrivateKey) ([]byte, error) {
	message, err := Bytes(bl.BlockHash)
	if err != nil {
		return nil, err
	}

	return ed25519.Sign(key, message), nil
}

func (bl *Block) VerifyBlockSign(key ed25519.PublicKey, signature []byte) (bool, error) {
	message, err := Bytes(bl.BlockHash)
	if err != nil {
		return false, err
	}
	return ed25519.Verify(key, message, signature), nil
}

func (bl *Block) Hash() (string, error) {
	if bl == nil {
		return "", errors.New("empty block")
	}
	b, err := Bytes(bl)
	if err != nil {
		return "", err
	}
	return Hash(b), nil
}
