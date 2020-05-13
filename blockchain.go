package bc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MSGBusLen             = 10
	MaxTransactionInBlock = 10
)

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
		validators:      sortValidators(genesis.Validators),
		lastBlockNum:    0,
		peers:           make(map[string]connectedPeer, 0),
		state:           State{users: make(map[string]uint64)},
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
	blocksMutex sync.RWMutex
	//peer address - > peer info
	peers      map[string]connectedPeer
	peersMutex sync.RWMutex
	//hash(state) - хеш от упорядоченного слайса ключ-значение
	//todo hash()
	state State

	validators      []ed25519.PublicKey
	validatorsMutex sync.Mutex
	//transaction hash - > transaction
	transactionPool   map[string]Transaction
	transactionsMutex sync.RWMutex

	//blockNum - > temporary block
	tmpBlocks      map[uint64][]Message
	tmpBlocksMutex sync.Mutex

	blockMessageMutex       sync.Mutex
	transactionMessageMutex sync.Mutex
}

func (c *Node) NodeKey() crypto.PublicKey {
	return c.key.Public()
}

func (c *Node) AmIValidatorNow() bool {
	pubKey, err := c.GetValidatorPubKey((c.GetLastBlockNum()) % c.GetValidatorsLength())
	if err != nil {
		return false
	}
	return uint64(bytes.Compare(pubKey, c.NodeKey().(ed25519.PublicKey))) == 0
}

func (c *Node) GetValidatorPubKey(index uint64) (ed25519.PublicKey, error) {
	c.validatorsMutex.Lock()
	defer c.validatorsMutex.Unlock()
	if index >= uint64(len(c.validators)) {
		return nil, errors.New("validator not exist")
	}

	pubKey := c.validators[index]

	return pubKey, nil
}

func (c *Node) GetValidatorsLength() uint64 {
	c.validatorsMutex.Lock()
	length := len(c.validators)
	c.validatorsMutex.Unlock()

	return uint64(length)
}

func (c *Node) Connection(address string, in chan Message, out chan Message) chan Message {
	if out == nil {
		out = make(chan Message, MSGBusLen)
	}

	c.peersMutex.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	c.peers[address] = connectedPeer{
		Address: address,
		Out:     out,
		In:      in,
		cancel:  cancel,
	}
	c.peersMutex.Unlock()

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
			BlockNum: c.GetLastBlockNum(),
		},
	})

	logger := make(chan error)

	for {
		select {
		case <-ctx.Done():
			log.Println("return")
			return
		case msg := <-peer.In:
			c.processMessage(ctx, peer.Address, msg, logger)
		case err := <-logger:
			if err != nil {
				log.Println("Process msg", err)
			}

			c.HandleErrors(ctx, err)
		}
	}
}

func (c *Node) processMessage(ctx context.Context, address string, msg Message, logger chan error) {
	switch m := msg.Data.(type) {
	//example
	case NodeInfoResp:
		go c.NodeInfoRespMessage(ctx, address, m, logger)
	case Transaction:
		go c.TransactionMessage(address, m, logger)
	case Block:
		go c.BlockMessage(ctx, address, m, logger)
	case NeedBlocks:
		go c.NeedBlocks(ctx, address, m, logger)
		//case DeleteMe:
		//	go c.DeleteMeMessage(m, logger, ctx)
	}
}

func (c *Node) Broadcast(ctx context.Context, msg Message) {
	c.peersMutex.RLock()
	for _, v := range c.peers {
		if v.Address != c.address {
			v.Send(ctx, msg)
		}
	}
	c.peersMutex.RUnlock()
}

func (c *Node) GetPeer(address string) (connectedPeer, error) {
	c.peersMutex.RLock()
	defer c.peersMutex.RUnlock()

	peer, ok := c.peers[address]
	if !ok {
		return connectedPeer{}, errors.New("peer not exist")
	}

	return peer, nil
}

func (c *Node) NeedBlocks(ctx context.Context, address string, message NeedBlocks, logger chan error) {
	peer, err := c.GetPeer(address)
	if err != nil {
		logger <- err
		return
	}

	fmt.Println(c.address, "connected to ", address, "to sync")

	for i := uint64(message) + 1; i <= c.GetLastBlockNum(); i++ {
		blockMessage := Message{
			From: c.NodeAddress(),
			Data: c.GetBlockByNumber(i),
		}
		peer.Send(ctx, blockMessage)
	}
}

func (c *Node) NodeInfoRespMessage(ctx context.Context, address string, message NodeInfoResp, logger chan error) {
	peer, err := c.GetPeer(address)
	if err != nil {
		logger <- err
		return
	}

	if c.GetLastBlockNum() < message.BlockNum {
		helpMessage := Message{
			From: c.NodeAddress(),
			Data: NeedBlocks(c.GetLastBlockNum()),
		}
		peer.Send(ctx, helpMessage)
	}
}

//func (c *Node) DeleteMeMessage(key DeleteMe, logger chan error, ctx context.Context) {
//	err := c.RemoveValidator(ed25519.PublicKey(key))
//	if err != nil {
//		logger <- err
//		return
//	}
//
//	c.SendRemoveValidator(ed25519.PublicKey(key), ctx)
//
//	//TODO refactoring
//	if c.AmIValidatorNow() {
//		err := c.CreateBlock(time.Now().Unix(), c.address, ctx)
//		if err != nil {
//			logger <- CreationBlockError{err.Error()}
//			return
//		}
//	}
//}

func (c *Node) TransactionMessage(address string, transaction Transaction, logger chan error) {
	c.transactionMessageMutex.Lock()
	defer c.transactionMessageMutex.Unlock()

	fmt.Println(address, "connected to ", c.address, " to offer transaction")

	hash, err := transaction.Hash()
	if err != nil {
		logger <- err
		return
	}
	if _, err := c.GetTransaction(hash); err == nil {
		logger <- errors.New("didn't send transaction to " + c.address + "already exist")
		return
	}
	if err := c.AddTransaction(transaction); err != nil {
		logger <- err
		return
	}

	fmt.Println(address, "sent transaction to ", c.address)
}

func (c *Node) BlockMessage(ctx context.Context, address string, block Block, logger chan error) {
	fmt.Println(address, "connected to ", c.address, " to offer block")
	c.BlockValidating(ctx, address, block, logger)

	for _, message := range c.GetAndRemoveFromTmpBlocks(c.GetLastBlockNum() + 1) {
		go c.BlockMessage(ctx, message.From, message.Data.(Block), logger)
	}

	return
}

func (c *Node) BlockValidating(ctx context.Context, address string, block Block, logger chan error) {
	c.blockMessageMutex.Lock()
	defer c.blockMessageMutex.Unlock()

	lastBlocNum := c.GetLastBlockNum()
	if block.BlockNum <= lastBlocNum {
		logger <- BlockMessageError{address + "didn't send block to " + c.address + "(offer declined), already exist"}
		return
	}
	if block.BlockNum > lastBlocNum+1 {
		c.SetTmpBlocks(block, address)

		logger <- OrderError{address: address, block: block}
		return
	}
	for _, v := range block.Transactions {
		if err := c.AddTransaction(v); err != nil {
			logger <- ValidationError{address, block.BlockNum, err}
			return
		}
	}

	validatorAddress, err := c.GetValidatorAddress()
	if err != nil {
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}

	tmpState := c.GetPieceOfState(validatorAddress, block.Transactions)

	handleErr := func(err error) {
		c.BackState(tmpState)
		logger <- ValidationError{address, block.BlockNum, err}
	}

	testBlock, err := c.BlockWithoutSign(lastBlocNum+1, block.Timestamp, block.Transactions, c.GetBlockByNumber(lastBlocNum).BlockHash, validatorAddress)
	if err != nil {
		handleErr(err)
		return
	}
	if testBlock.BlockHash != block.BlockHash {
		handleErr(errors.New("hashes not equal"))
		return
	}

	pubKey, err := c.GetValidatorPubKey((block.BlockNum - 1) % c.GetValidatorsLength())
	if err != nil {
		handleErr(err)
		return
	}

	signVerify, err := block.VerifyBlockSign(pubKey)
	if err != nil {
		handleErr(err)
		return
	}
	if !signVerify {
		handleErr(errors.New("signs not equal"))
		return
	}
	if err := c.insertBlock(block); err != nil {
		handleErr(err)
		return
	}

	fmt.Println(address, "sent block to ", c.address)
	go c.Broadcast(ctx, Message{address, block})

	if c.AmIValidatorNow() {
		// todo shadowing
		err = c.CreateBlock(ctx, time.Now().Unix(), c.address)
		if err != nil {
			logger <- CreationBlockError{err.Error()}
			return
		}
	}
}

func (c *Node) BackState(tmpState map[string]uint64) {
	c.state.Lock()

	for name, money := range tmpState {
		c.state.users[name] = money
	}

	c.state.Unlock()
}

func (c *Node) GetAndRemoveFromTmpBlocks(numOfBlock uint64) (msgs []Message) {
	c.tmpBlocksMutex.Lock()

	msgs = c.tmpBlocks[numOfBlock]
	delete(c.tmpBlocks, numOfBlock)

	c.tmpBlocksMutex.Unlock()
	return msgs
}

func (c *Node) SetTmpBlocks(block Block, address string) {
	c.tmpBlocksMutex.Lock()

	c.tmpBlocks[block.BlockNum] = append(c.tmpBlocks[block.BlockNum], Message{
		From: address,
		Data: block,
	})

	c.tmpBlocksMutex.Unlock()
}

func (c *Node) GetBalance(account string) (uint64, error) {
	c.state.RLock()
defer c.state.RUnlock()

	balance, ok := c.state.users[account]
	if !ok {
		return 0, errors.New("unknown user")
	}

	return balance, nil
}

func (c *Node) GetTransaction(hash string) (Transaction, error) {
	c.transactionsMutex.RLock()
	defer c.transactionsMutex.RUnlock()

	if tr, ok := c.transactionPool[hash]; ok {
		return tr, nil
	}
	return nil, errors.New("unknown transaction")
}

func (c *Node) DeleteTransaction(hash string) {
	c.transactionsMutex.Lock()
	delete(c.transactionPool, hash)
	c.transactionsMutex.Unlock()
}

func (c *Node) AddTransaction(transaction Transaction) error {
	c.transactionsMutex.Lock()
	defer c.transactionsMutex.Unlock()

	if err := transaction.Verify(); err != nil {
		return err
	}

	hash, err := transaction.Hash()
	if err != nil {
		return err
	}

	c.transactionPool[hash] = transaction

	return nil
}

func (c *Node) GetBlockByNumber(blockNum uint64) Block {
	c.blocksMutex.RLock()
	block := c.blocks[blockNum]
	c.blocksMutex.RUnlock()

	return block
}

func (c *Node) NodeInfo() NodeInfoResp {
	panic("implement me")
}

func (c *Node) NodeAddress() string {
	return c.address
}

func (c *Node) CreateBlock(ctx context.Context, timestamp int64, address string) error {
	transactions, err := c.PrepareTransactions()
	if err != nil {
		return nil
	}

	lastBlockNum := c.GetLastBlockNum()
	prevBlockHash := c.GetBlockByNumber(lastBlockNum).BlockHash

	block, err := c.BlockWithoutSign(lastBlockNum+1, timestamp, transactions, prevBlockHash, address)
	if err != nil {
		return nil
	}

	if err = block.SignBlock(c.key); err != nil {
		return nil
	}

	if err = c.insertBlock(block); err != nil {
		return nil
	}

	message := Message{
		From: c.NodeAddress(),
		Data: block,
	}
	go c.Broadcast(ctx, message)

	return nil
}

func (c *Node) BlockWithoutSign(blockNum uint64, timestamp int64, transactions []Transaction, prevBlockHash, address string) (Block, error) {
	block := Block{
		BlockNum:      blockNum,
		Timestamp:     timestamp,
		Transactions:  transactions,
		BlockHash:     "",
		PrevBlockHash: prevBlockHash,
		StateHash:     "",
		Signature:     nil,
	}

	err := c.CountStateAfterBlock(address, transactions)
	if err != nil {
		return Block{}, err
	}

	bytes, err := Bytes(c.state)
	if err != nil {
		return Block{}, err
	}

	stateHash := Hash(bytes)

	block.StateHash = stateHash
	block.BlockHash, err = block.Hash()
	if err != nil {
		return Block{}, err
	}

	return block, nil
}

func (c *Node) GetPieceOfState(address string, transactions []Transaction) map[string]uint64 {
	c.state.Lock()

	tmpState := make(map[string]uint64, len(c.state.users))
	for _, tr := range transactions {
		for _, user := range tr.GetUsers() {
			tmpState[user] = c.state.users[user]
		}
	}
	tmpState[address] = c.state.users[address]

	c.state.Unlock()
	return tmpState
}

func (c *Node) GetValidatorAddress() (string, error) {
	pubKey, err := c.GetValidatorPubKey((c.GetLastBlockNum()) % c.GetValidatorsLength())
	if err != nil {
		return "", err
	}

	address, err := PubKeyToAddress(pubKey)
	if err != nil {
		return "", err
	}

	return address, nil
}

func (c *Node) CountStateAfterBlock(address string, transactions []Transaction) error {
	c.state.Lock()
	defer c.state.Unlock()

	for _, v := range transactions {
		if err := v.Execute(address, &c.state); err != nil {
			return err
		}
	}

	c.state.users[address] += 1000

	return nil
}

func (c *Node) insertGenesis() {
	block := c.genesis.ToBlock()
	c.state.Lock()

	for _, v := range block.Transactions {
		c.state.users[v.(*Transfer).To] = c.state.users[v.(*Transfer).To] + v.(*Transfer).Amount
		c.state.users[c.address] += v.(*Transfer).Fee
	}

	c.state.Unlock()
	c.AddBlock(block)
}

func (c *Node) insertBlock(b Block) error {
	for _, v := range b.Transactions {
		hash, _ := v.Hash()
		c.DeleteTransaction(hash)
	}

	c.AddLastBlockNum()
	c.AddBlock(b)
	return nil
}

func (c *Node) PrepareTransactions() ([]Transaction, error) {
	c.transactionsMutex.RLock()

	var transactions []Transaction

	i := MaxTransactionInBlock
	for _, tr := range c.transactionPool {
		from := tr.GetSenderAddress()

		balance, err := c.GetBalance(from)
		if err != nil {
			log.Println(err)
		}
		if balance < tr.GetPrice() {
			log.Println("user " + from + " has insufficient balance")
		}

		transactions = append(transactions, tr)

		i--
		if i == 0 {
			break
		}
	}

	c.transactionsMutex.RUnlock()
	return transactions, nil
}

func (c *Node) HandleErrors(ctx context.Context, err error) {
	switch e := err.(type) {
	case BlockMessageError:
		return
	case ValidationError:
		//TODO send that block was declined with error
	case OrderError:
		msg := Message{
			From: e.address,
			Data: e.block,
		}

		go c.Broadcast(ctx, msg)
	case CreationBlockError:
		panic(err)
	}
}

func (c *Node) GetLastBlockNum() uint64 {
	return atomic.LoadUint64(&c.lastBlockNum)
}

func (c *Node) AddLastBlockNum() {
	atomic.AddUint64(&c.lastBlockNum, 1)
}

func (c *Node) AddBlock(block Block) {
	c.blocksMutex.Lock()
	c.blocks = append(c.blocks, block)
	c.blocksMutex.Unlock()
}

func (c *Node) FindValidatorByPubKey(key ed25519.PublicKey) (uint64, error) {
	index := sort.Search(len(c.validators), func(i int) bool {
		return bytes.Compare(c.validators[i], key) >= 0
	})
	if index == len(c.validators) {
		return 0, errors.New("validator not exist")
	}

	return uint64(index), nil
}

func (c *Node) RemoveValidator(key ed25519.PublicKey) error {
	c.validatorsMutex.Lock()
	defer c.validatorsMutex.Unlock()

	index, err := c.FindValidatorByPubKey(key)
	if err != nil {
		return err
	}

	c.validators = append(c.validators[:index], c.validators[index+1:]...)
	return nil
}

//func (c *Node) SendRemoveValidator(key ed25519.PublicKey, ctx context.Context) {
//	message := Message{
//		From: c.address,
//		Data: DeleteMe(key),
//	}
//
//	go c.Broadcast(ctx, message)
//}
