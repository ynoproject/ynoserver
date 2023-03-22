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

package assets

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Assets struct {
	MapIds []int

	SpriteNames       map[string]bool
	SystemNames       map[string]bool
	SoundNames        map[string]bool
	IgnoredSoundNames map[string]bool
	PictureNames      map[string]bool
	PicturePrefixes   []string
	BattleAnimIds     map[int]bool
}

func GetAssets(gamePath string) *Assets {
	return &Assets{
		MapIds: getMaps(gamePath),

		SpriteNames: getCharSets(gamePath),
		SystemNames: getSystems(gamePath),
		SoundNames:  getSounds(gamePath),
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

	return a.SpriteNames[name]
}

func (a *Assets) IsValidSystem(name string, ignoreSingleQuotes bool) bool {
	if ignoreSingleQuotes {
		name = strings.ReplaceAll(name, "'", "")
	}

	return a.SystemNames[name]
}

func (a *Assets) IsValidSound(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	if a.IgnoredSoundNames[name] {
		return false
	}

	return a.SoundNames[name]
}

func (a *Assets) IsValidPicture(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	if a.PictureNames[name] {
		return true
	}

	for _, prefix := range a.PicturePrefixes {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return true
		}
	}

	return false
}

func (a *Assets) IsValid2kkiSprite(name string, room int) bool {
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
