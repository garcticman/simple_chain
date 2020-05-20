package bc

import "crypto"

type Blockchain interface {
	NodeKey() crypto.PublicKey
	NodeAddress() string
	Connection(address string, in chan Message, out chan Message) chan Message

	PublicAPI
}

type PublicAPI interface {
	//network
	AddPeer(Blockchain) error
	//RemovePeer(Blockchain) error

	//for clients
	GetBalance(account string) (uint64, error)
	//add to transaction pool
	AddTransaction(transaction Transaction) error
	//SignTransaction(transaction Transaction) (Transaction, error)

	//sync
	GetBlockByNumber(ID uint64) (Block, error)
	NodeInfo() NodeInfoResp
}
