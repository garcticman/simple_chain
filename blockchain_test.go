package bc

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"fmt"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
	"time"
)

func InitForTest(numOfPeers, numOfValidators int) ([]*Node, error) {

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

		peers[i].insertGenesis()
	}

	return peers, nil
}

func TestRemoveValidator(t *testing.T) {
	peer := Node{validators: make([]ed25519.PublicKey, 3)}
	pubKeys := make([]crypto.PublicKey, 3)

	for i := range peer.validators {
		pubKeys[i], _, _ = ed25519.GenerateKey(nil)
	}

	peer.validators = sortValidators(pubKeys)

	publicKey := peer.validators[2]
	peer.RemoveValidator(publicKey)
	for _, v := range peer.validators {
		if reflect.DeepEqual(v, publicKey) {
			t.Fail()
		}
	}
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

	tr := Transfer{
		From:      address,
		To:        "ABC",
		Amount:    0,
		Fee:       1,
		PubKey:    key.Public().(ed25519.PublicKey),
		Signature: nil,
	}

	tr.SignTransaction(key)

	err = peer.AddTransaction(&tr)
	if err != nil {
		t.Error(err)
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
			address, err := peer.GetValidatorAddress()

			err = peer.CreateBlock(context.TODO(), time.Now().Unix(), address)
			if err != nil {
				t.Error(err)
			}
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
			err := node.CreateBlock(context.TODO(), time.Now().Unix(), node.address)
			if err != nil {
				t.Error(err)
			}

			break
		}
	}

	//time.Sleep(time.Second)
	//pubKey := peers[2].NodeKey().(ed25519.PublicKey)
	//err = peers[2].RemoveValidator(pubKey)
	//if err != nil {
	//	t.Error(err)
	//}

	//peers[2].SendRemoveValidator(pubKey, context.TODO())
	//time.Sleep(time.Second)
	//
	//for _, peer := range peers {
	//	peer.validatorsMutex.Lock()
	//	if _, err := peer.FindValidatorByPubKey(pubKey); err == nil {
	//		t.Fail()
	//	}
	//	peer.validatorsMutex.Unlock()
	//}

	tr := Transfer{
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

	if err := peers[3].AddTransaction(&tr); err != nil {
		t.Error(err)
	}

	context := context.Background()
	message := Message{
		From: peers[3].NodeAddress(),
		Data: Transaction(&tr),
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
