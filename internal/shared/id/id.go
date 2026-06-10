package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

func New(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", fmt.Errorf("id prefix is required")
	}

	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return prefix + "_" + hex.EncodeToString(bytes[:]), nil
}
