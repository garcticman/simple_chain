package bc

import (
	"context"
	"crypto"
	"fmt"
	"sort"
	"time"
)

// first block with blockchain settings
type Genesis struct {
	//Account -> funds
	Alloc map[string]uint64
	//list of validators public keys
	Validators []crypto.PublicKey
}

func (g Genesis) ToBlock() Block {
	transactions := make([]Transaction, len(g.Alloc))

	i := 0
	for key, amount := range g.Alloc {
		transactions[i] = Transaction{
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
		return transactions[i].To < transactions[j].To
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
