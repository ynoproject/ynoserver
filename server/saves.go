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

package server

import (
	"errors"
	"os"
	"time"

	"github.com/klauspost/compress/zstd"
)

func getSaveDataTimestamp(playerUuid string) (time.Time, error) { // called by api only
	info, err := os.Stat("saves/" + serverConfig.GameName + "/" + playerUuid + ".osd")
	if err != nil {
		//return time.UnixMilli(0), nil // HACK: no error return because it breaks forest-orb

		// remove this later
		var timestamp time.Time
		err = db.QueryRow("SELECT timestamp FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, serverConfig.GameName).Scan(&timestamp)
		if err != nil {
			return timestamp, err
		}

		return timestamp, nil
	}

	return info.ModTime(), nil
}

func getSaveData(playerUuid string) ([]byte, error) { // called by api only
	file, err := os.ReadFile("saves/" + serverConfig.GameName + "/" + playerUuid + ".osd")
	if err != nil {
		//return nil, err

		// remove this later
		var saveData string
		err = db.QueryRow("SELECT data FROM playerGameSaves WHERE uuid = ? AND game = ?", playerUuid, serverConfig.GameName).Scan(&saveData)
		if err != nil {
			return nil, err
		}

		return []byte(saveData), nil
	}

	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}

	decompressed, err := dec.DecodeAll(file, []byte{})
	if err != nil {
		return nil, err
	}

	defer dec.Close()

	return decompressed, nil
}

func createGameSaveData(playerUuid string, data []byte) error { // called by api only
	// remove this later
	if len(data) > 1 && data[0] == '{' {
		return errors.New("old format save data is no longer supported")
	}

	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}

	defer enc.Close()

	os.WriteFile("saves/" + serverConfig.GameName + "/" + playerUuid + ".osd", enc.EncodeAll(data, []byte{}), 0644)

	return nil
}

func clearGameSaveData(playerUuid string) error { // called by api only
	return os.Remove("saves/" + serverConfig.GameName + "/" + playerUuid + ".osd")
}
