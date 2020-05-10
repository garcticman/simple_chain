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
	//тут была ошибка файлового дескриптора, при записи в файл
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(v)
}

func ReadFromJSON(filename string, data interface{}) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	//немного упростить можно
	return json.NewDecoder(file).Decode(data)
}
