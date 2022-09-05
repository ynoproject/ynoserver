package security

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"math/rand"
	"time"
)

func GenerateKey() uint32 {
	rand.Seed(time.Now().UnixNano())
	return rand.Uint32()
}

func VerifySignature(key uint32, signKey []byte, msg []byte) bool {
	byteKey := make([]byte, 4)

	binary.BigEndian.PutUint32(byteKey, key)

	hash := sha1.New()
	hash.Write(signKey)
	hash.Write(byteKey)
	hash.Write(msg[4:])

	return bytes.Equal(hash.Sum(nil)[:4], msg[:4])
}

func VerifyCounter(counter *uint32, msg []byte) bool {
	if cnt := binary.BigEndian.Uint32(msg[4:len(msg) - 4]); *counter < cnt {
		*counter = cnt
		return true
	}

	return false
}
