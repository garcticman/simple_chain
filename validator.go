package bc

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"sort"
	"sync"
)

type Validators struct {
	pubKeys []ed25519.PublicKey
	sync.RWMutex
}

func (v *Validators) RemoveValidator(key ed25519.PublicKey) error {
	v.Lock()
	defer v.Unlock()

	index, err := v.FindValidatorByPubKey(key)
	if err != nil {
		return err
	}

	v.pubKeys = append(v.pubKeys[:index], v.pubKeys[index+1:]...)
	return nil
}

func (v *Validators) FindValidatorByPubKey(key ed25519.PublicKey) (uint64, error) {
	index := sort.Search(len(v.pubKeys), func(i int) bool {
		return bytes.Compare(key, v.pubKeys[i]) <= 0
	})
	if index == len(v.pubKeys) || bytes.Compare(key, v.pubKeys[index]) != 0 {
		return 0, errors.New("validator not exist")
	}

	return uint64(index), nil
}

func (v *Validators) GetValidatorPubKey(index uint64) (ed25519.PublicKey, error) {
	v.RLock()
	defer v.RUnlock()

	if index >= uint64(len(v.pubKeys)) {
		return nil, errors.New("validator not exist")
	}

	pubKey := v.pubKeys[index]

	return pubKey, nil
}

func (v *Validators) GetValidatorsLength() uint64 {
	v.RLock()
	length := len(v.pubKeys)
	v.RUnlock()

	return uint64(length)
}

func (v *Validators) GetValidatorAddress(lastBlockNum uint64) (string, error) {
	pubKey, err := v.GetValidatorPubKey((lastBlockNum) % v.GetValidatorsLength())
	if err != nil {
		return "", err
	}

	address, err := PubKeyToAddress(pubKey)
	if err != nil {
		return "", err
	}

	return address, nil
}

func (v *Validators) Add(pubKeys []ed25519.PublicKey) {
	v.Lock()
	v.pubKeys = append(v.pubKeys, pubKeys...)
	v.pubKeys = sortValidators(v.pubKeys)
	v.Unlock()
}
