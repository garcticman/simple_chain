package blockchain

import (
	"crypto/ed25519"
	"crypto/rand"
	"simple_bchain/simple_chain"
	"time"
)

const (
	hashLength = 32
)

func createBlock(transactions []bc.Transaction, pervBlockHash string) *bc.Block {
	block := &bc.Block{
		BlockNum:      getLatestBlock() + 1,
		Timestamp:     time.Now().Unix(),
		Transactions:  transactions,
		BlockHash:     "",
		PrevBlockHash: pervBlockHash,
		StateHash:     "",
		Signature:     nil,
	}

	var err error
	block.BlockHash, err = block.Hash()
	if err != nil{
		panic(err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}

	block.Signature, err = createSignature(block.BlockHash, privateKey)
	if err != nil{
		panic(err)
	}

	if ok := ed25519.Verify(publicKey, []byte(block.BlockHash), block.Signature); ok == false {
		panic("not success")
	}

	return block
}

func getLatestBlock() uint64 {
	return 0
}

func createSignature(blockHash string, privateKey ed25519.PrivateKey) ([]byte, error) {
	byteHash := []byte(blockHash)
/*
	if len(byteHash) != hashLength {
		return nil, errors.New("hash length must be 32")
	}
*/
	return ed25519.Sign(privateKey, byteHash), nil
}


