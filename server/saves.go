/*
	Copyright (C) 2021-2024  The YNOproject Developers

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

package server

import (
	"os"
	"time"

	"github.com/klauspost/compress/zstd"
)

func getSaveDataTimestamp(playerUuid string) (time.Time, error) { // called by api only
	info, err := os.Stat("saves/" + config.gameName + "/" + playerUuid + ".osd")
	if err != nil {
		return time.UnixMilli(0), nil // HACK: no error return because it breaks forest-orb
	}

	return info.ModTime().UTC(), nil
}

func getSaveData(playerUuid string) ([]byte, error) { // called by api only
	file, err := os.ReadFile("saves/" + config.gameName + "/" + playerUuid + ".osd")
	if err != nil {
		return nil, err
	}

	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}

	defer dec.Close()

	decompressed, err := dec.DecodeAll(file, []byte{})
	if err != nil {
		return nil, err
	}

	return decompressed, nil
}

func createGameSaveData(playerUuid string, data []byte) error { // called by api only
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}

	defer enc.Close()

	os.WriteFile("saves/"+config.gameName+"/"+playerUuid+".osd", enc.EncodeAll(data, []byte{}), 0644)

	return nil
}

func clearGameSaveData(playerUuid string) error { // called by api only
	return os.Remove("saves/" + config.gameName + "/" + playerUuid + ".osd")
}
