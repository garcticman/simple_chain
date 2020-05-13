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
}

func LoadGenesis() (genesis Genesis, err error) {
	tempGenesis := struct {
		//Account -> funds
		Alloc map[string]uint64
		//list of validators public keys
		Validators []ed25519.PublicKey
	}{}

	err = ReadFromJSON("Genesis.json", &tempGenesis)
	if err != nil {
		// todo: не уверен, что это нужно и полезно.
		return
		// client := github.NewClient(nil)
		// context := context.Background()
		//
		// var reader io.ReadCloser
		// reader, err = client.Repositories.DownloadContents(context, "garcticman", "simple_chain", "Genesis.json", nil)
		// if err != nil {
		// 	return
		// }
		//
		// var file []byte
		// file, err = ioutil.ReadAll(reader)
		// if err != nil {
		// 	return
		// }
		//
		// if err := json.Unmarshal(file, &tempGenesis); err != nil {
		// 	return Genesis{}, err
		// }
		//
		// err = SaveToJSON(tempGenesis, "Genesis.json")
		// if err != nil {
		// 	return
		// }
	}

	//to have ed25519.PublicKey in crypto.PublicKey interface
	genesis = Genesis{
		Alloc:      tempGenesis.Alloc,
		Validators: make([]crypto.PublicKey, len(tempGenesis.Validators)),
	}

	for i, v := range tempGenesis.Validators {
		genesis.Validators[i] = v
	}

	return
}

func (bl *Block) SignBlock(key ed25519.PrivateKey) error {
	message, err := Bytes(bl.BlockHash)
	if err != nil {
		return err
	}

	bl.Signature = ed25519.Sign(key, message)
	return nil
}

func (bl *Block) VerifyBlockSign(key ed25519.PublicKey) (bool, error) {
	message, err := Bytes(bl.BlockHash)
	if err != nil {
		return false, err
	}
	return ed25519.Verify(key, message, bl.Signature), nil
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
