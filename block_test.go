package bc

//func TestCreateNewGenesis(t *testing.T) {
//	numOfPeers := 5
//	numOfValidators := 3
//	initialBalance := uint64(100000)
//
//	genesis := Genesis{
//		&ChainConfig{TwoSigns: 20},
//		make(map[string]uint64),
//		make([]crypto.PublicKey, 0, numOfValidators),
//	}
//
//	keys := make([]ed25519.PrivateKey, numOfPeers)
//	for i := range keys {
//		_, key, err := ed25519.GenerateKey(nil)
//		if err != nil {
//			t.Fatal(err)
//		}
//		keys[i] = key
//		if numOfValidators > 0 {
//			genesis.Validators = append(genesis.Validators, key.Public())
//			numOfValidators--
//		}
//
//		address, err := PubKeyToAddress(key.Public())
//		if err != nil {
//			t.Error(err)
//		}
//		genesis.Alloc[address] = initialBalance
//
//		if err := SaveToJSON(key, fmt.Sprintf("Keys/peer_%d.json", i)); err != nil {
//			t.Error(err)
//		}
//	}
//
//	if err := SaveToJSON(genesis, "Genesis.json"); err != nil {
//		t.Error(err)
//	}
//}
//
//func TestLoadGenesis(t *testing.T) {
//	genesis, err := LoadGenesis()
//	if err != nil {
//		t.Error(err)
//	}
//
//	t.Log(genesis)
//}
