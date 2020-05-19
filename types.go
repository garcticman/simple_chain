package bc

import (
	"context"
	"crypto"
	"fmt"
	"sort"
	"sync"
	"time"
)

type State struct {
	users map[string]uint64
	sync.RWMutex
}

type TransactionNonce struct {
	usersNonce map[string]uint64
	sync.RWMutex
}

func (tn *TransactionNonce) GetNonce(address string) uint64 {
	tn.RLock()
	nonce := tn.usersNonce[address]
	tn.RUnlock()

	return nonce
}

func (tn *TransactionNonce) AddNonce(address string) {
	tn.Lock()
	tn.usersNonce[address]++
	tn.Unlock()
}

func (tn *TransactionNonce) Compare(address string, nonce uint64) (res int8) {
	tn.Lock()
	switch true {
	case tn.usersNonce[address]+1 < nonce:
		res = -1
	case tn.usersNonce[address] > nonce:
		res = 1
	default:
	}
	tn.Unlock()

	return
}

// first block with blockchain settings
type Genesis struct {
	ChainConfig *ChainConfig
	//Account -> funds
	Alloc map[string]uint64
	//list of validators public keys
	Validators []crypto.PublicKey
}

func (g Genesis) ToBlock() Block {
	transactions := make([]Transaction, len(g.Alloc))

	i := 0
	for key, amount := range g.Alloc {
		transactions[i] = &Transfer{
			From:      "",
			To:        key,
			Amount:    amount,
			Fee:       0,
			PubKey:    nil,
			Signature: nil,
		}
		i++
	}
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].(*Transfer).To < transactions[j].(*Transfer).To
	})

	block := Block{
		BlockNum:      0,
		Timestamp:     0,
		Transactions:  transactions,
		BlockHash:     "",
		PrevBlockHash: "",
		StateHash:     "",
		Signature:     nil,
	}

	var err error
	block.BlockHash, err = block.Hash()
	if err != nil {
		panic(err)
	}

	return block
}

type Message struct {
	From string
	Data interface{}
}

type NodeInfoResp struct {
	NodeName string
	BlockNum uint64
}

type NeedBlocks uint64

type connectedPeer struct {
	Address string
	In      chan Message
	Out     chan Message
	cancel  context.CancelFunc
}

func (cp connectedPeer) Send(ctx context.Context, m Message) {
	//todo timeout using context + done check
	for {
		select {
		case <-time.After(time.Second):
			fmt.Println("timeout")
			cp.cancel()
			return
		case cp.Out <- m:
			return
		default:
		}
	}
	//cp.Out <- m
}
