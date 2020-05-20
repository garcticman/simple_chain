package bc

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"fmt"
	"github.com/stretchr/testify/require"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func InitForTest(numOfPeers, numOfValidators int) ([]*Node, error) {

	genesis := Genesis{
		&ChainConfig{TwoSigns: 20},
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
		if numOfValidators > 0 {
			genesis.Validators = append(genesis.Validators, key.Public())
			numOfValidators--
		}

		address, err := PubKeyToAddress(key.Public())
		if err != nil {
			return nil, err
		}
		genesis.Alloc[address] = 1000
	}

	var err error
	for i := 0; i < numOfPeers; i++ {
		peers[i], err = NewNode(keys[i], genesis)
		if err != nil {
			return nil, err
		}

		peers[i].InsertGenesis()
	}

	return peers, nil
}

func TestRemoveValidator(t *testing.T) {
	peer := Node{validators: Validators{pubKeys: make([]ed25519.PublicKey, 3)}}
	pubKeys := make([]crypto.PublicKey, 3)

	for i := range peer.validators.pubKeys {
		pubKeys[i], _, _ = ed25519.GenerateKey(nil)
	}

	peer.validators.pubKeys = sortValidators(convertValidators(pubKeys))

	publicKey := peer.validators.pubKeys[2]
	peer.validators.RemoveValidator(publicKey)
	for _, v := range peer.validators.pubKeys {
		if reflect.DeepEqual(v, publicKey) {
			t.Fail()
		}
	}
}

func TestRemoveValidatorWithAmIValidator(t *testing.T) {
	peers, err := InitForTest(5, 5)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range peers {
		publicKey := v.validators.pubKeys[2]
		v.validators.RemoveValidator(publicKey)
	}

	var i int
	for _, v := range peers {
		if v.AmIValidatorNow(v.lastBlockNum) {
			i++
		}
	}

	require.Equal(t, 1, i)
}

func TestAddTransaction(t *testing.T) {
	peer := Node{transactionPool: map[string]Transaction{}}

	err := peer.AddTransaction(&Transfer{
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

	err = peer.AddTransaction(&Transfer{
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

	var tr Transaction

	tr = &Transfer{
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
		t.Fatal(err)
	}
}

func TestAmIValidatorNow(t *testing.T) {
	peers, err := InitForTest(3, 3)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		for _, peer := range peers {
			if peer.AmIValidatorNow(uint64(i)) {
				if err != nil {
					t.Fatal(err)
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

func TestStartingSync(t *testing.T) {
	peers, err := InitForTest(3, 3)
	if err != nil {
		t.Fatal(err)
	}

	for _, peer := range peers {
		if peer.AmIValidatorNow(peer.GetLastBlockNum()) {
			address, err := peer.validators.GetValidatorAddress(peer.GetLastBlockNum())

			err = peer.CreateBlock(context.TODO(), time.Now().Unix(), address)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			err := peers[i].AddPeer(peers[j])
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	time.Sleep(time.Second)

	block1, _ := peers[1].GetBlockByNumber(1)
	block2, _ := peers[0].GetBlockByNumber(1)
	require.Equal(t, block1, block2)

	block1, _ = peers[2].GetBlockByNumber(1)
	block2, _ = peers[0].GetBlockByNumber(1)
	require.Equal(t, block1, block2)
}

func TestHardFork(t *testing.T) {
	peers := make([]*Node, 5)
	var err error

	for i := 0; i < 5; i++ {

		peers[i], err = MyNode(fmt.Sprintf("peer_%d", i))

		peers[i].InsertGenesis()
	}

	for i := 0; i < len(peers); i++ {
		for j := i + 1; j < len(peers); j++ {
			err = peers[i].AddPeer(peers[j])
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, node := range peers {
		if node.AmIValidatorNow(node.GetLastBlockNum()) {
			err := node.CreateBlock(context.TODO(), time.Now().Unix(), node.address)
			if err != nil {
				t.Fatal(err)
			}

			break
		}
	}

	time.Sleep(time.Second * 2)
	//fork check
	block, _ := peers[0].GetBlockByNumber(19)

	require.Nil(t, block.SignatureTwo)
	for i := uint64(20); i <= 40; i++ {
		block20, _ := peers[0].GetBlockByNumber(i)

		pubKey, err := peers[0].validators.GetValidatorPubKey(i % 3)
		if err != nil {
			t.Fatal(err)
		}

		verifyResult, err := block20.VerifyBlockSign(pubKey, block20.SignatureTwo)
		if err != nil {
			t.Fatal(err)
		}

		if !verifyResult {
			t.Fatal("hard-fork error")
		}
	}
}

func TestStartingBlockchain(t *testing.T) {
	var err error
	initialBalance := uint64(10000)

	peers := make([]*Node, 5)
	for i := 0; i < 5; i++ {

		peers[i], err = MyNode(fmt.Sprintf("peer_%d", i))

		peers[i].InsertGenesis()
	}

	for i := 0; i < len(peers); i++ {
		for j := i + 1; j < len(peers); j++ {
			err = peers[i].AddPeer(peers[j])
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, node := range peers {
		if node.AmIValidatorNow(node.GetLastBlockNum()) {
			err := node.CreateBlock(context.TODO(), time.Now().Unix(), node.address)
			if err != nil {
				t.Fatal(err)
			}

			break
		}
	}

	time.Sleep(time.Second)
	pubKey := peers[2].NodeKey().(ed25519.PublicKey)

	var tr Transaction
	tr = &DeleteMe{
		From:      peers[2].address,
		Fee:       1,
		PubKey:    pubKey,
		Signature: nil,
		Nonce:     peers[2].TransactionNonce.GetNonce(peers[2].address),
	}

	err = tr.SignTransaction(peers[2].key)
	if err != nil {
		t.Fatal(err)
	}

	if err := peers[2].AddTransaction(tr); err != nil {
		t.Fatal(err)
	}

	context := context.Background()
	message := Message{
		From: peers[2].NodeAddress(),
		Data: tr,
	}

	peers[2].Broadcast(context, message)

	time.Sleep(time.Second)

	for _, peer := range peers {
		peer.validators.Lock()
		if _, err := peer.validators.FindValidatorByPubKey(pubKey); err == nil {
			t.Fatal("Validator not removed")
		}
		peer.validators.Unlock()
	}

	tr = &Transfer{
		From:   peers[3].NodeAddress(),
		To:     peers[4].NodeAddress(),
		Amount: 100,
		Fee:    10,
		PubKey: peers[3].NodeKey().(ed25519.PublicKey),
		Nonce:  peers[3].TransactionNonce.GetNonce(peers[3].address),
	}

	err = tr.SignTransaction(peers[3].key)
	if err != nil {
		t.Fatal(err)
	}

	if err := peers[3].AddTransaction(tr); err != nil {
		t.Fatal(err)
	}

	message = Message{
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
		t.Fatal("Incorrect from balance " + strconv.FormatUint(balance, 10))
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
	peers[0].validators.Lock()
	for _, pubKey := range peers[0].validators.pubKeys {
		address, _ := PubKeyToAddress(pubKey)
		balance, err = peers[0].GetBalance(address)
		if err != nil {
			t.Fatal(err)
		}

		if balance < initialBalance {
			t.Fatal("Incorrect validator balance")
		}
	}
}
