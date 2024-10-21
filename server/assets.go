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
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Assets struct {
	maps []int

	sprites  map[string]bool
	systems  map[string]bool
	sounds   map[string]bool
	pictures map[string]bool
}

func getAssets(gamePath string) *Assets {
	return &Assets{
		maps: getMaps(gamePath),

		sprites:  getCharSets(gamePath),
		systems:  getSystems(gamePath),
		sounds:   getSounds(gamePath),
		pictures: getPictures(gamePath),
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
	root := gamePath + "/Sound"
	sounds := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			path, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			path = path[:len(path)-len(filepath.Ext(path))]
			sounds[path] = true
		}
		return err
	})
	if err != nil {
		panic(err)
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

func getPictures(gamePath string) map[string]bool {
	files, err := os.ReadDir(gamePath + "/Picture")
	if err != nil {
		panic(err)
	}

	pictures := make(map[string]bool)
	for _, file := range files {
		pictures[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return pictures
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

	return a.sprites[name]
}

func (a *Assets) IsValidSystem(name string, ignoreSingleQuotes bool) bool {
	if ignoreSingleQuotes {
		name = strings.ReplaceAll(name, "'", "")
	}

	return a.systems[name]
}

func (a *Assets) IsValidSound(name string) bool {
	if strings.Contains(name, "../") || strings.Contains(name, "..\\") {
		return false
	}
	if config.badSounds[name] {
		return false
	}

	valid := a.sounds[name]
	if !valid && filepath.Separator == '/' {
		// check for either backward or forward slash
		alias := strings.Replace(name, "\\", "/", 1)
		valid = a.sounds[alias]
		// cache the results
		a.sounds[name] = valid
	}
	return valid
}

func (a *Assets) IsValidPicture(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	if !a.pictures[name] {
		return false
	}

	if config.pictures[name] {
		return true
	}

	for _, prefix := range config.picturePrefixes {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return true
		}
	}

	return false
}

// Yume 2kki

func isValid2kkiSprite(name string, room int) bool {
	if (allowed2kkiSprites[name] ||
		(strings.Contains(name, "syujinkou") ||
			strings.Contains(name, "effect") ||
			strings.Contains(name, "yukihitsuji_game") ||
			strings.Contains(name, "zenmaigaharaten_kisekae") ||
			strings.Contains(name, "主人公"))) &&
		!(strings.Contains(name, "zenmaigaharaten_kisekae") && room != 176) {
		return true
	}

	return true
}

var allowed2kkiSprites = map[string]bool{
	"#null":                   true,
	"kodomo_04-1":             true,
	"Kong_Urotsuki_CharsetFC": true,
	"kura CharSet01":          true,
	"kuro9-8":                 true,
	"natl_char_uro":           true,
	"nuls_sujinkou":           true,
	"RioCharset16":            true,
	"urotsuki_sniper":         true,
	"urotsuki_Swimsuit01":     true,
	"urotsuki_Swimsuit02":     true,
	"urotsuki_taoru":          true,
}
