package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

func uniqueID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%x", prefix, b), nil
}

func randomSeed() (int64, error) {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 53))
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}
