package bc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

const MSGBusLen = 100

func NewNode(key ed25519.PrivateKey, genesis Genesis) (*Node, error) {
	address, err := PubKeyToAddress(key.Public())
	if err != nil {
		return nil, err
	}

	node := &Node{
		key:             key,
		address:         address,
		genesis:         genesis,
		blocks:          make([]Block, 0),
		validators:      convertValidators(genesis.Validators),
		lastBlockNum:    0,
		peers:           make(map[string]connectedPeer, 0),
		state:           make(map[string]uint64),
		transactionPool: make(map[string]Transaction),
		tmpBlocks:       make(map[uint64][]Message),
	}

	return node, err
}

type Node struct {
	key          ed25519.PrivateKey
	address      string
	genesis      Genesis
	lastBlockNum uint64

	//state
	blocks      []Block
	blocksMutex sync.Mutex
	//peer address - > peer info
	peers map[string]connectedPeer
	//hash(state) - хеш от упорядоченного слайса ключ-значение
	//todo hash()
	state      map[string]uint64
	validators []ed25519.PublicKey

	//transaction hash - > transaction
	transactionPool   map[string]Transaction
	transactionsMutex sync.Mutex

	//blockNum - > temporary block
	tmpBlocks map[uint64][]Message
}

func (c *Node) NodeKey() crypto.PublicKey {
	return c.key.Public()
}

func (c *Node) AmIValidatorNow() bool {
	return uint64(bytes.Compare(c.validators[(c.lastBlockNum)%uint64(len(c.validators))], c.NodeKey().(ed25519.PublicKey))) == 0
}

func (c *Node) Connection(address string, in chan Message, out chan Message) chan Message {
	if out == nil {
		out = make(chan Message, MSGBusLen)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.peers[address] = connectedPeer{
		Address: address,
		Out:     out,
		In:      in,
		cancel:  cancel,
	}

	go c.peerLoop(ctx, c.peers[address])
	return c.peers[address].Out
}

func (c *Node) AddPeer(peer Blockchain) error {
	remoteAddress, err := PubKeyToAddress(peer.NodeKey())
	if err != nil {
		return err
	}

	if c.address == remoteAddress {
		return errors.New("self connection")
	}

	if _, ok := c.peers[remoteAddress]; ok {
		return nil
	}

	out := make(chan Message, MSGBusLen)
	in := peer.Connection(c.address, out, nil)
	c.Connection(remoteAddress, in, out)
	return nil
}

func (c *Node) peerLoop(ctx context.Context, peer connectedPeer) {
	//todo handshake
	peer.Send(ctx, Message{
		From: c.NodeAddress(),
		Data: NodeInfoResp{
			NodeName: c.address,
			BlockNum: c.lastBlockNum,
		},
	})

	logger := make(chan error)

	for {
		select {
		case <-ctx.Done():
			log.Println("return")
			return
		case msg := <-peer.In:
			c.processMessage(peer.Address, msg, ctx, logger)

		//broadcast to connected peers
		//c.Broadcast(ctx, msg)
		case msg := <-logger:
			if msg != nil {
				log.Println("Process msg", msg)
			}
		}
	}
}

func (c *Node) processMessage(address string, msg Message, ctx context.Context, logger chan error) {
	switch m := msg.Data.(type) {
	//example
	case NodeInfoResp:
		go c.NodeInfoRespMessage(address, m, ctx)
	case Transaction:
		go c.TransactionMessage(address, m, logger)
	case Block:
		go c.BlockMessage(address, m, logger, ctx)
	case NeedBlocks:
		go c.NeedBlocks(address, m, ctx)
	}
	//TODO send that block was declined with error
}

func (c *Node) Broadcast(ctx context.Context, msg Message) {
	for _, v := range c.peers {
		if v.Address != c.address {
			v.Send(ctx, msg)
		}
	}
}

func (c *Node) NeedBlocks(address string, message NeedBlocks, ctx context.Context) {
	fmt.Println(c.address, "connected to ", address, "to sync")

	c.blocksMutex.Lock()
	defer c.blocksMutex.Unlock()

	for i := uint64(message) + 1; i <= c.lastBlockNum; i++ {
		blockMessage := Message{
			From: c.NodeAddress(),
			Data: c.blocks[i],
		}
		c.peers[address].Send(ctx, blockMessage)
	}
}

func (c *Node) NodeInfoRespMessage(address string, message NodeInfoResp, ctx context.Context) {

	c.blocksMutex.Lock()
	defer c.blocksMutex.Unlock()

	if c.lastBlockNum < message.BlockNum {
		helpMessage := Message{
			From: c.NodeAddress(),
			Data: NeedBlocks(c.lastBlockNum),
		}
		c.peers[address].Send(ctx, helpMessage)
	}
}

func (c *Node) TransactionMessage(address string, transaction Transaction, logger chan error) {
	fmt.Println(address, "connected to ", c.address, " to offer transaction")

	c.transactionsMutex.Lock()
	defer c.transactionsMutex.Unlock()

	hash, err := transaction.Hash()
	if err != nil {
		logger <- err
		return
	}
	if _, ok := c.transactionPool[hash]; ok {
		logger <- errors.New("didn't send transaction to " + c.address + "(offer declined)")
		return
	}
	if err := c.AddTransaction(transaction); err != nil {
		logger <- err
		return
	}

	fmt.Println(address, "sent transaction to ", c.address)
}

func (c *Node) BlockMessage(address string, block Block, logger chan error, ctx context.Context) {
	fmt.Println(address, "connected to ", c.address, " to offer block")
	c.BlockValidating(address, block, logger, ctx)

	for _, message := range c.tmpBlocks[c.lastBlockNum + 1] {
		go c.BlockMessage(message.From, message.Data.(Block), logger, ctx)
	}

	return
}

func (c *Node) BlockValidating(address string, block Block, logger chan error, ctx context.Context) {
	c.blocksMutex.Lock()
	defer c.blocksMutex.Unlock()

	if block.BlockNum <= c.lastBlockNum {
		logger <- BlockMessageError{address + "didn't send block to " + c.address + "(offer declined), already exist"}
		return
	}
	if block.BlockNum > c.lastBlockNum+1 {
		c.tmpBlocks[block.BlockNum] = append(c.tmpBlocks[block.BlockNum], Message{
			From: address,
			Data: block,
		})

		logger <- OrderError{address: address, block: block}
		return
	}
	for _, v := range block.Transactions {
		if err := c.AddTransaction(v); err != nil {
			logger <- ValidationError{address, block.BlockNum, err}
			return
		}
	}

	testBlock, err := c.BlockWithoutSign(c.lastBlockNum+1, block.Timestamp, block.Transactions, c.GetBlockByNumber(c.lastBlockNum).BlockHash)
	if err != nil {
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}
	if testBlock.BlockHash != block.BlockHash {
		logger <- ValidationError{address, block.BlockNum, errors.New("hashes not equal")}
		return
	}
	signVerify, err := block.VerifyBlockSign(c.validators[(block.BlockNum-1)%uint64(len(c.validators))])
	if err != nil {
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}
	if !signVerify {
		logger <- ValidationError{address, block.BlockNum, errors.New("signs not equal")}
		return
	}
	if err := c.insertBlock(block); err != nil {
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}

	fmt.Println(address, "sent block to ", c.address)
	go c.Broadcast(ctx, Message{address, block})

	if c.AmIValidatorNow() {
		transactions, err := c.PrepareTransactions()
		if err != nil {
			logger <- CreationBlockError{err.Error()}
			return
		}

		block, err := c.CreateBlock(c.lastBlockNum+1, time.Now().Unix(), transactions, c.GetBlockByNumber(c.lastBlockNum).BlockHash)
		if err != nil {
			logger <- CreationBlockError{err.Error()}
			return
		}

		if err := c.insertBlock(block); err != nil {
			logger <- CreationBlockError{err.Error()}
			return
		}

		message := Message{
			From: c.NodeAddress(),
			Data: block,
		}
		go c.Broadcast(ctx, message)
	}
}

func (c *Node) RemovePeer(peer Blockchain) error {
	panic("implement me")
	return nil
}

func (c *Node) GetBalance(account string) (uint64, error) {
	balance, ok := c.state[account]
	if !ok {
		return 0, errors.New("unknown user")
	}

	return balance, nil
}

func (c *Node) AddTransaction(transaction Transaction) error {
	tr := transaction
	tr.Signature = []byte{}

	b, err := tr.Bytes()
	if err != nil {
		return err
	}
	if address, err := PubKeyToAddress(transaction.PubKey); address != transaction.From || err != nil {
		return err
	}
	if !ed25519.Verify(transaction.PubKey, b, transaction.Signature) {
		return errors.New("wrong signature")
	}

	hash, err := transaction.Hash()
	if err != nil {
		return err
	}

	c.transactionPool[hash] = transaction

	return nil
}

func (c *Node) GetBlockByNumber(ID uint64) Block {
	if ID >= uint64(len(c.blocks)) {
		return Block{}
	}

	return c.blocks[ID]
}

func (c *Node) NodeInfo() NodeInfoResp {
	panic("implement me")
}

func (c *Node) NodeAddress() string {
	return c.address
}

func (c *Node) SignTransaction(transaction Transaction) (Transaction, error) {
	b, err := transaction.Bytes()
	if err != nil {
		return Transaction{}, err
	}

	transaction.Signature = ed25519.Sign(c.key, b)
	return transaction, nil
}

func (c Node) CreateBlock(blockNum uint64, timestamp int64, transactions []Transaction, prevBlockHash string) (Block, error) {
	block, err := c.BlockWithoutSign(blockNum, timestamp, transactions, prevBlockHash)
	if err != nil {
		return Block{}, err
	}

	if err := block.SignBlock(c.key); err != nil {
		return Block{}, err
	}

	return block, nil
}

func (c Node) BlockWithoutSign(blockNum uint64, timestamp int64, transactions []Transaction, prevBlockHash string) (Block, error) {
	block := Block{
		BlockNum:      blockNum,
		Timestamp:     timestamp,
		Transactions:  transactions,
		BlockHash:     "",
		PrevBlockHash: prevBlockHash,
		StateHash:     "",
		Signature:     nil,
	}

	state, err := c.CountStateAfterBlock(transactions)
	if err != nil {
		return Block{}, err
	}

	bytes, err := Bytes(state)
	if err != nil {
		return Block{}, err
	}

	stateHash, err := Hash(bytes)
	if err != nil {
		return Block{}, err
	}

	block.StateHash = stateHash
	block.BlockHash, err = block.Hash()
	if err != nil {
		return Block{}, err
	}

	return block, nil
}

func (c Node) CountStateAfterBlock(transactions []Transaction) (map[string]uint64, error) {
	state := make(map[string]uint64, len(c.state))

	for i, v := range c.state {
		state[i] = v
	}

	address, err := PubKeyToAddress(c.validators[(c.lastBlockNum)%uint64(len(c.validators))])
	if err != nil {
		return nil, err
	}
	for _, v := range transactions {
		if _, ok := state[v.From]; !ok {
			return nil, errors.New("user " + v.From + " not exist")
		}
		if _, ok := state[v.To]; !ok {
			return nil, errors.New("user " + v.To + " not exist")
		}
		if state[v.From] < v.Amount+v.Fee {
			return nil, errors.New("user " + v.From + " has lack of balance")
		}

		state[v.From] = c.state[v.From] - v.Amount - v.Fee
		state[v.To] = c.state[v.To] + v.Amount
		state[address] += v.Fee
	}
	state[address] += 1000

	return state, nil
}

func (c *Node) insertGenesis() {
	block := c.genesis.ToBlock()

	for _, v := range block.Transactions {
		c.state[v.To] = c.state[v.To] + v.Amount
		c.state[c.address] += v.Fee
	}

	c.blocks = append(c.blocks, block)
}

func (c *Node) insertBlock(b Block) error {
	address, err := PubKeyToAddress(c.validators[(c.lastBlockNum)%uint64(len(c.validators))])
	if err != nil {
		return err
	}

	for _, v := range b.Transactions {
		c.state[v.From] = c.state[v.From] - v.Amount - v.Fee
		c.state[v.To] = c.state[v.To] + v.Amount
		c.state[address] += v.Fee

		hash, _ := v.Hash()
		delete(c.transactionPool, hash)
	}
	c.state[address] += 1000

	c.lastBlockNum = b.BlockNum
	c.blocks = append(c.blocks, b)
	return nil
}

func (c *Node) PrepareTransactions() ([]Transaction, error) {
	var transactions []Transaction

	i := 10
	for _, tr := range c.transactionPool {
		balance, err := c.GetBalance(tr.From)
		if err != nil {
			log.Println(err)
		}
		if balance < tr.Amount+tr.Fee {
			log.Println("user " + tr.From + " has insufficient balance")
		}

		transactions = append(transactions, tr)

		i--
		if i == 0 {
			return transactions, nil
		}
	}

	return transactions, nil
}
