package bc

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
)

func PubKeyToAddress(key crypto.PublicKey) (string, error) {
	if v, ok := key.(ed25519.PublicKey); ok {
		b := sha256.Sum256(v)
		return hex.EncodeToString(b[:]), nil
	}
	return "", errors.New("incorrect key")
}

func Hash(b []byte) (string, error) {
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:]), nil
}

func Bytes(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func SaveToJSON(v interface{}, filename string) error {
	file, err := os.OpenFile(filename, os.O_CREATE, os.ModePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.Encode(v)
	//
	//bytes, _ := json.Marshal(v)
	//if err := ioutil.WriteFile(filename, bytes, 0644); err != nil {
	//	return err
	//}

	return nil
}

func ReadFromJSON(filename string, data interface{}) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	//decoder.Token()

	for decoder.More() {
		decoder.Decode(data)
	}

	return nil
}
