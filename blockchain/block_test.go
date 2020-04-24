package blockchain

import (
	"crypto/sha256"
	bc "github.com/youricorocks/simple_chain"
	"testing"
)

func Test_createBlock(t *testing.T) {
	testPrevHash := sha256.Sum256([]byte{})
	block := createBlock([]bc.Transaction{}, string(testPrevHash[:]))

	t.Log([]byte(block.BlockHash))
	t.Log(block.Signature)
}
