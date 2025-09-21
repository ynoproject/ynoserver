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
	"bytes"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"
)

func getSaveDataTimestamp(playerUuid string) (time.Time, error) { // called by api only
	info, err := os.Stat(filepath.Join("saves", config.gameName, playerUuid+".osd"))
	if err != nil {
		return time.UnixMilli(0), nil // HACK: no error return because it breaks forest-orb
	}

	return info.ModTime().UTC(), nil
}

// it's the caller's responsibility to close the returned Decoder
func getSaveData(playerUuid string) (io.Reader, error) { // called by api only
	f, err := os.Open(filepath.Join("saves", config.gameName, playerUuid+".osd"))
	if err != nil {
		return nil, err
	}

	defer f.Close()

	zd, err := zstd.NewReader(f)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, zd)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func createGameSaveData(playerUuid string, data io.Reader) error { // called by api only
	f, err := os.OpenFile(filepath.Join("saves", config.gameName, playerUuid+".osd"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 06444)
	if err != nil {
		return err
	}

	defer f.Close()

	ze, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		return err
	}

	defer ze.Close()

	_, err = io.Copy(ze, data)
	if err != nil {
		return err
	}

	return nil
}

func clearGameSaveData(playerUuid string) error { // called by api only
	return os.Remove(filepath.Join("saves", config.gameName, playerUuid+".osd"))
}
