/*
	Copyright (C) 2021-2023  The YNOproject Developers

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
	"path/filepath"
	"strconv"
	"strings"
)

type Assets struct {
	mapIds []int

	spriteNames       map[string]bool
	systemNames       map[string]bool
	soundNames        map[string]bool
	ignoredSoundNames map[string]bool
	pictureNames      map[string]bool
	picturePrefixes   []string
	battleAnimIds     map[int]bool
}

func getAssets(gamePath string) *Assets {
	return &Assets{
		mapIds: getMaps(gamePath),

		spriteNames: getCharSets(gamePath),
		systemNames: getSystems(gamePath),
		soundNames:  getSounds(gamePath),
	}
}

func getCharSets(gamePath string) map[string]bool {
	files, err := os.ReadDir(gamePath + "/CharSet")
	if err != nil {
		panic(err)
	}

	charSets := make(map[string]bool)
	for _, file := range files {
		charSets[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return charSets
}

func getSounds(gamePath string) map[string]bool {
	files, err := os.ReadDir(gamePath + "/Sound")
	if err != nil {
		panic(err)
	}

	sounds := make(map[string]bool)
	for _, file := range files {
		sounds[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return sounds
}

func getSystems(gamePath string) map[string]bool {
	files, err := os.ReadDir(gamePath + "/System")
	if err != nil {
		panic(err)
	}

	systems := make(map[string]bool)
	for _, file := range files {
		systems[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return systems
}

func getMaps(gamePath string) []int {
	files, err := os.ReadDir(gamePath + "/")
	if err != nil {
		panic(err)
	}

	var maps []int
	for _, file := range files {
		if len(file.Name()) == 11 && file.Name()[7:] == ".lmu" {
			id, err := strconv.Atoi(file.Name()[3:7])
			if err != nil {
				panic(err)
			}

			maps = append(maps, id)
		}
	}

	return maps
}

func (a *Assets) IsValidSprite(name string) bool {
	if name == "" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	return a.spriteNames[name]
}

func (a *Assets) IsValidSystem(name string, ignoreSingleQuotes bool) bool {
	if ignoreSingleQuotes {
		name = strings.ReplaceAll(name, "'", "")
	}

	return a.systemNames[name]
}

func (a *Assets) IsValidSound(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	if a.ignoredSoundNames[name] {
		return false
	}

	return a.soundNames[name]
}

func (a *Assets) IsValidPicture(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	if a.pictureNames[name] {
		return true
	}

	for _, prefix := range a.picturePrefixes {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return true
		}
	}

	return false
}
