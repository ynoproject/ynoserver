/*
	Copyright (C) 2021-2022  The YNOproject Developers

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

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

func verifyCounter(counter *uint32, msg []byte) bool {
	if cnt := binary.BigEndian.Uint32(msg[4 : len(msg)-4]); *counter < cnt {
		*counter = cnt
		return true
	}

	return false
}
