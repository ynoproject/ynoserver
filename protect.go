package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"math/rand"
	"time"
)

func generateKey() uint32 {
	rand.Seed(time.Now().UnixNano())
	return rand.Uint32()
}

func verifySignature(key uint32, msg []byte) bool {
	byteKey := make([]byte, 4)

	binary.BigEndian.PutUint32(byteKey, key)

	hash := sha1.New()
	hash.Write(config.signKey)
	hash.Write(byteKey)
	hash.Write(msg[4:])

	return bytes.Equal(hash.Sum(nil)[:4], msg[:4])
}

func verifyCounter(counter *uint16, msg []byte) bool {
	if cnt := binary.BigEndian.Uint16(msg[4:len(msg) - 2]); *counter < cnt {
		*counter = cnt
		return true
	}

	return false
}
