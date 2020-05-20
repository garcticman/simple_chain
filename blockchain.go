package bc

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"errors"
	"log"
	"runtime"
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
		chainConfig:      genesis.ChainConfig,
		key:              key,
		address:          address,
		genesis:          genesis,
		blocks:           make([]Block, 0),
		validators:       Validators{pubKeys: sortValidators(convertValidators(genesis.Validators))},
		lastBlockNum:     0,
		peers:            make(map[string]connectedPeer, 0),
		state:            State{users: make(map[string]uint64)},
		transactionPool:  make(map[string]Transaction),
		tmpBlocks:        make(map[uint64][]Message),
		TransactionNonce: TransactionNonce{usersNonce: make(map[string]uint64)},
	}

	return node, err
}

type Node struct {
	chainConfig *ChainConfig

	key          ed25519.PrivateKey
	address      string
	genesis      Genesis
	lastBlockNum uint64
	nonce        uint64

	TransactionNonce TransactionNonce

	//state
	blocks      []Block
	blocksMutex sync.RWMutex
	//peer address - > peer info
	peers      map[string]connectedPeer
	peersMutex sync.RWMutex
	//hash(state) - хеш от упорядоченного слайса ключ-значение
	state State

	validators Validators
	//transaction hash - > transaction
	transactionPool   map[string]Transaction
	transactionsMutex sync.RWMutex

	//blockNum - > temporary block
	tmpBlocks      map[uint64][]Message
	tmpBlocksMutex sync.Mutex

	blockMessageMutex       sync.Mutex
	transactionMessageMutex sync.Mutex
}

func (c *Node) PrivateKey() ed25519.PrivateKey {
	return c.key
}

func (c *Node) NodeKey() crypto.PublicKey {
	return c.key.Public()
}

func (c *Node) AmIValidatorNow(lastBlockNum uint64) bool {
	pubKey, err := c.validators.GetValidatorPubKey(lastBlockNum % c.validators.GetValidatorsLength())
	if err != nil {
		return false
	}
	return uint64(bytes.Compare(pubKey, c.NodeKey().(ed25519.PublicKey))) == 0
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
				//log.Println("Process msg", err)
			}

			c.HandleErrors(ctx, err)
		}
	}
}

func (c *Node) processMessage(ctx context.Context, address string, msg Message, logger chan error) {
	//time.Sleep(time.Millisecond)

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

	for i := uint64(message) + 1; i <= c.GetLastBlockNum(); i++ {
		block, err := c.GetBlockByNumber(i)
		if err != nil {
			logger <- err
			return
		}

		blockMessage := Message{
			From: c.NodeAddress(),
			Data: block,
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

func (c *Node) TransactionMessage(address string, transaction Transaction, logger chan error) {
	c.transactionMessageMutex.Lock()
	defer c.transactionMessageMutex.Unlock()

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
}

func (c *Node) BlockMessage(ctx context.Context, address string, block Block, logger chan error) {
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

	pubKey, err := c.validators.GetValidatorPubKey((block.BlockNum - 1) % c.validators.GetValidatorsLength())
	if err != nil {
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}
	signVerify, err := block.VerifyBlockSign(pubKey, block.Signature)
	if err != nil {
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}
	if !signVerify {
		logger <- ValidationError{address, block.BlockNum, errors.New("signs not equal")}
		return
	}

	if IsForked(c.chainConfig.TwoSigns, block.BlockNum) {
		if c.AmIValidatorNow(lastBlocNum + 1) {
			block.SignatureTwo, err = block.SignBlock(c.key)
			if block.SignatureTwo == nil {
				runtime.Breakpoint()
			}
			if err != nil {
				logger <- ValidationError{address, block.BlockNum, err}
				return
			}
		} else {
			if block.SignatureTwo == nil {
				logger <- TwoSignsError{address, block}
				return
			}

			pubKey, err := c.validators.GetValidatorPubKey((block.BlockNum) % c.validators.GetValidatorsLength())
			if err != nil {
				logger <- ValidationError{address, block.BlockNum, err}
				return
			}

			signVerify, err := block.VerifyBlockSign(pubKey, block.SignatureTwo)
			if err != nil {
				logger <- ValidationError{address, block.BlockNum, err}
				return
			}
			if !signVerify {
				logger <- ValidationError{address, block.BlockNum, errors.New("signs not equal")}
				return
			}
		}
	}

	for _, v := range block.Transactions {
		if err := c.AddTransaction(v); err != nil {
			logger <- ValidationError{address, block.BlockNum, err}
			return
		}
	}
	validatorAddress, err := c.validators.GetValidatorAddress(lastBlocNum)
	if err != nil {
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}

	tmpState, tmpValidators := c.GetPieceOfStateAndValidators(validatorAddress, block.Transactions)
	handleErr := func(err error) {
		c.RollBack(tmpState, tmpValidators)
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}
	prevBlock, err := c.GetBlockByNumber(lastBlocNum)
	if err != nil {
		c.RollBack(tmpState, tmpValidators)
		logger <- ValidationError{address, block.BlockNum, err}
		return
	}

	testBlock, err := c.BlockWithoutSign(lastBlocNum+1, block.Timestamp, block.Transactions, prevBlock.BlockHash, validatorAddress)
	if err != nil {
		handleErr(err)
		return
	}
	if testBlock.BlockHash != block.BlockHash {
		handleErr(err)
		return
	}
	if err := c.insertBlock(ctx, block); err != nil {
		handleErr(err)
		return
	}

	go c.Broadcast(ctx, Message{address, block})

	if c.AmIValidatorNow(c.GetLastBlockNum()) {
		err := c.CreateBlock(ctx, time.Now().Unix(), c.address)
		if err != nil {
			logger <- CreationBlockError{err.Error()}
			return
		}
	}
}

func (c *Node) RollBack(tmpState map[string]uint64, tmpValidators []ed25519.PublicKey) {
	c.state.Lock()
	for name, money := range tmpState {
		c.state.users[name] = money
	}
	c.state.Unlock()

	c.validators.Add(tmpValidators)
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

	if c.TransactionNonce.Compare(transaction.GetSenderAddress(), transaction.GetNonce()) == 1 {
		return nil
	}

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

func (c *Node) GetBlockByNumber(blockNum uint64) (Block, error) {
	c.blocksMutex.RLock()
	defer c.blocksMutex.RUnlock()

	if blockNum >= uint64(len(c.blocks)) {
		return Block{}, errors.New("block not exist")
	}
	block := c.blocks[blockNum]

	return block, nil
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

	prevBlock, err := c.GetBlockByNumber(lastBlockNum)
	if err != nil {
		return err
	}
	prevBlockHash := prevBlock.BlockHash

	tmpState, tmpValidators := c.GetPieceOfStateAndValidators(c.address, transactions)

	block, err := c.BlockWithoutSign(lastBlockNum+1, timestamp, transactions, prevBlockHash, address)
	if err != nil {
		c.RollBack(tmpState, tmpValidators)
		return nil
	}

	block.Signature, err = block.SignBlock(c.key)
	if err != nil {
		c.RollBack(tmpState, tmpValidators)
		return nil
	}

	//TODO убрать применение блока если форк случился
	//if !IsForked(c.chainConfig.TwoSigns, block.BlockNum) {
	if err := c.insertBlock(ctx, block); err != nil {
		c.RollBack(tmpState, tmpValidators)
		return nil
	}
	//}

	message := Message{
		From: c.NodeAddress(),
		Data: block,
	}
	go c.Broadcast(ctx, message)

	if c.AmIValidatorNow(c.GetLastBlockNum()) {
		err := c.CreateBlock(ctx, time.Now().Unix(), c.address)
		if err != nil {
			return err
		}
	}

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

func (c *Node) GetPieceOfStateAndValidators(address string, transactions []Transaction) (map[string]uint64, []ed25519.PublicKey) {
	c.state.Lock()
	defer c.state.Unlock()

	tmpState := make(map[string]uint64)
	var tmpValidators []ed25519.PublicKey
	for _, tr := range transactions {
		users, validator := tr.GetUsersAndRemovedValidator()

		for _, user := range users {
			tmpState[user] = c.state.users[user]
		}

		if validator != nil {
			tmpValidators = append(tmpValidators, validator)
		}
	}
	tmpState[address] = c.state.users[address]

	return tmpState, tmpValidators
}

func (c *Node) CountStateAfterBlock(address string, transactions []Transaction) error {
	c.state.Lock()
	defer c.state.Unlock()

	for _, v := range transactions {
		if err := v.Execute(address, &c.state.users, &c.validators); err != nil {
			return err
		}
	}

	c.state.users[address] += 1000

	return nil
}

func (c *Node) InsertGenesis() {
	block := c.genesis.ToBlock()
	c.state.Lock()

	for _, v := range block.Transactions {
		c.state.users[v.(*Transfer).To] = c.state.users[v.(*Transfer).To] + v.(*Transfer).Amount
		c.state.users[c.address] += v.(*Transfer).Fee
	}

	c.state.Unlock()
	c.AddBlock(block)
}

func (c *Node) insertBlock(ctx context.Context, b Block) error {
	for _, tr := range b.Transactions {
		hash, err := tr.Hash()
		if err != nil {
			return err
		}

		c.TransactionNonce.SetNonce(tr.GetSenderAddress(), tr.GetNonce())
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

		if c.TransactionNonce.Compare(from, tr.GetNonce()) < 0 {
			continue
		}

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
	case TwoSignsError:
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
