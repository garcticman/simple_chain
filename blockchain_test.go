package bc

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
	"time"
)

func InitForTest(numOfPeers, numOfValidators int) ([]*Node, error) {
	numOfValidatorsCopy:=numOfValidators
	genesis := Genesis{
		make(map[string]uint64),
		make([]crypto.PublicKey, 0, numOfPeers),
	}

	peers := make([]*Node, numOfPeers)
	keys := make([]ed25519.PrivateKey, numOfPeers)
	for i := range keys {
		_, key, err := ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}
		keys[i] = key
		if numOfValidatorsCopy > 0 {
			genesis.Validators = append(genesis.Validators, key.Public())
			numOfValidatorsCopy--
		}

		address, err := PubKeyToAddress(key.Public())
		if err != nil {
			return nil, err
		}
		genesis.Alloc[address] = 1000
	}

	var err error
	//panic
	for i := 0; i < numOfValidators; i++ {
		peers[i], err = NewNode(keys[i], genesis)
		if err != nil {
			return nil, err
		}

		peers[i].insertGenesis()
	}

	return peers, nil
}

func TestAddTransaction(t *testing.T) {
	peer := Node{transactionPool: map[string]Transaction{}}

	err := peer.AddTransaction(Transaction{
		From:      "",
		To:        "",
		Amount:    0,
		Fee:       10,
		PubKey:    nil,
		Signature: nil,
	})
	if err == nil {
		t.Fail()
	}

	_, key, _ := ed25519.GenerateKey(nil)
	address, _ := PubKeyToAddress(key.Public())

	err = peer.AddTransaction(Transaction{
		From:      address,
		To:        "ABC",
		Amount:    0,
		Fee:       0,
		PubKey:    nil,
		Signature: nil,
	})
	if err == nil {
		t.Fail()
	}

	tr := Transaction{
		From:      address,
		To:        "ABC",
		Amount:    0,
		Fee:       1,
		PubKey:    key.Public().(ed25519.PublicKey),
		Signature: nil,
	}

	tr.SignTransaction(key)

	err = peer.AddTransaction(tr)
	if err != nil {
		t.Error(err)
	}
}
func TestAddTransaction2(t *testing.T) {
	peers,err:= InitForTest(2,2)
	if err != nil {
		t.Fatal(err)
	}
	err = peers[0].AddPeer(peers[1])
	if err != nil {
		t.Fatal(err)
	}

	tr := Transaction{
		From:      peers[0].address,
		To:        peers[1].address,
		Amount:    10,
		Fee:       1,
		PubKey:    peers[0].key.Public().(ed25519.PublicKey),
		Signature: nil,
	}

	err = tr.SignTransaction(peers[0].key)
	if err != nil {
		t.Fatal(err)
	}

	hash,err:=tr.Hash()
	if err != nil {
		t.Fatal(err)
	}


	err = peers[0].AddTransaction(tr)
	if err != nil {
		t.Error(err)
	}
	time.Sleep(time.Second)
	tr2,err:=peers[1].GetTransaction(hash)
	if err!=nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tr, tr2) {
		t.Fatal(tr, tr2)
	}
}

func TestBlockMessage(t *testing.T) {
	peers, err := InitForTest(3, 3)
	if err != nil {
		t.Error(err)
	}

	block := Block{
		BlockNum:      1,
		Timestamp:     time.Now().Unix(),
		Transactions:  nil,
		BlockHash:     "",
		PrevBlockHash: "",
		StateHash:     "",
		Signature:     nil,
	}

	logger := make(chan error, 1)

	peers[1].BlockMessage(peers[0].address, block, logger, context.TODO())
	if !reflect.DeepEqual(<-logger, ValidationError{address: peers[0].address, numOfBlock: block.BlockNum, error: errors.New("hashes not equal")}) {
		t.Fail()
	}

	block.PrevBlockHash = peers[0].blocks[0].BlockHash
	peers[0].state[peers[0].address] += 1000

	bytes, _ := Bytes(peers[0].state)
	block.StateHash, _ = Hash(bytes)
	block.BlockHash, _ = block.Hash()
	if err := block.SignBlock(peers[0].key); err != nil {
		t.Error(err)
	}

	peers[1].BlockMessage(peers[0].address, block, logger, context.TODO())
	select {
	case <-logger:
		t.Error(err)
	default:
	}
	peers[2].BlockMessage(peers[0].address, block, logger, context.TODO())
	select {
	case <-logger:
		t.Error(err)
	default:
	}

	peers[2].BlockMessage(peers[1].address, peers[1].GetBlockByNumber(2), logger, context.TODO())
	select {
	case <-logger:
		t.Error(err)
	default:
	}

	if len(peers[2].blocks) != 4 {
		t.Fail()
	}

	if peers[2].GetState()[peers[0].address] != 2000 && peers[2].GetState()[peers[1].address] != 2000 && peers[2].GetState()[peers[2].address] != 2000 {
		t.Fail()
	}
}

func TestAmIValidatorNow(t *testing.T) {
	peers, err := InitForTest(3, 3)
	if err != nil {
		t.Error(err)
	}

	for i := 0; i < 10; i++ {
		for _, peer := range peers {
			if peer.AmIValidatorNow() {
				if err != nil {
					t.Error(err)
				}
				peers[0].blocks = append(peers[0].blocks, Block{})
				peers[1].blocks = append(peers[1].blocks, Block{})
				peers[2].blocks = append(peers[2].blocks, Block{})
				break
			}
		}
	}

	require.Equal(t, 11, len(peers[0].blocks))
	require.Equal(t, 11, len(peers[1].blocks))
	require.Equal(t, 11, len(peers[2].blocks))
}

func TestSync(t *testing.T) {
	peers, err := InitForTest(3, 3)
	if err != nil {
		t.Error(err)
	}

	for _, peer := range peers {
		if peer.AmIValidatorNow() {
			block, err := peer.CreateBlock(peer.GetLastBlockNum()+1, time.Now().Unix(), nil, peer.GetBlockByNumber(0).BlockHash)
			if err != nil {
				t.Error(err)
			}
			peer.insertBlock(block)
			break
		}
	}

	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			err := peers[i].AddPeer(peers[j])
			if err != nil {
				t.Error(err)
			}
		}
	}

	time.Sleep(time.Second)

	require.Equal(t, peers[1].GetBlockByNumber(1), peers[0].GetBlockByNumber(1))
	require.Equal(t, peers[2].GetBlockByNumber(1), peers[0].GetBlockByNumber(1))
}

func TestStartingBlockchain(t *testing.T) {
	var err error
	initialBalance := uint64(10000)

	peers := make([]*Node, 5)
	for i := 0; i < 5; i++ {

		peers[i], err = MyNode(fmt.Sprintf("peer_%d", i))

		peers[i].insertGenesis()
	}

	for i := 0; i < len(peers); i++ {
		for j := i + 1; j < len(peers); j++ {
			err = peers[i].AddPeer(peers[j])
			if err != nil {
				t.Error(err)
			}
		}
	}

	for _, node := range peers {
		if node.AmIValidatorNow() {
			transactions, err := node.PrepareTransactions()
			if err != nil {
				t.Error(err)
			}

			block, err := node.CreateBlock(node.lastBlockNum+1, time.Now().Unix(), transactions, node.GetBlockByNumber(node.lastBlockNum).BlockHash)
			if err != nil {
				t.Error(err)
			}

			if err := node.insertBlock(block); err != nil {
				t.Error(err)
			}
			context := context.Background()
			message := Message{
				From: node.NodeAddress(),
				Data: block,
			}

			for _, peer := range node.peers {
				peer.Send(context, message)
			}

			break
		}
	}

	tr := Transaction{
		From:   peers[3].NodeAddress(),
		To:     peers[4].NodeAddress(),
		Amount: 100,
		Fee:    10,
		PubKey: peers[3].NodeKey().(ed25519.PublicKey),
	}

	err = tr.SignTransaction(peers[3].key)
	if err != nil {
		t.Fatal(err)
	}

	if err := peers[3].AddTransaction(tr); err != nil {
		t.Error(err)
	}

	context := context.Background()
	message := Message{
		From: peers[3].NodeAddress(),
		Data: tr,
	}

	peers[3].Broadcast(context, message)

	time.Sleep(time.Second)

	balance, err := peers[0].GetBalance(peers[3].NodeAddress())
	if err != nil {
		t.Fatal(err)
	}

	if (balance+110)%1000 != 0 {
		t.Fatal("Incorrect from balance")
	}

	//check "to" balance
	balance, err = peers[0].GetBalance(peers[4].NodeAddress())
	if err != nil {
		t.Fatal(err)
	}

	if (balance-100)%1000 != 0 {
		t.Fatal("Incorrect to balance")
	}

	//check validators balance
	balance, err = peers[0].GetBalance(peers[1].NodeAddress())
	if err != nil {
		t.Error(err)
	}
	if balance < initialBalance {
		t.Error("Incorrect validator balance")
	}

	//check validators balance
	balance, err = peers[0].GetBalance(peers[2].NodeAddress())
	if err != nil {
		t.Error(err)
	}
	if balance < initialBalance {
		t.Error("Incorrect validator balance")
	}

	//check validators balance
	balance, err = peers[0].GetBalance(peers[4].NodeAddress())
	if err != nil {
		t.Error(err)
	}
	if balance < initialBalance {
		t.Error("Incorrect validator balance")
	}
}
