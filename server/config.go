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
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	gameName string
	gamePath string

	spRooms         []int
	badSounds       map[string]bool
	pictures        map[string]bool
	picturePrefixes []string
	battleAnimIds   map[int]bool

	ipHubKey string

	logging struct {
		maxSize    int
		maxBackups int
		maxAge     int
	}
}

type ConfigFile struct {
	GameName string `yaml:"game_name"`
	GamePath string `yaml:"game_path"`

	SpRooms         string `yaml:"sp_rooms"`
	BadSounds       string `yaml:"bad_sounds"`
	PictureNames    string `yaml:"picture_names"`
	PicturePrefixes string `yaml:"picture_prefixes"`
	BattleAnimIds   string `yaml:"battle_anim_ids"`

	SignKey  string `yaml:"sign_key"`
	IpHubKey string `yaml:"iphub_key"`

	Logging struct {
		MaxSize    int `yaml:"max_size"`
		MaxBackups int `yaml:"max_backups"`
		MaxAge     int `yaml:"max_age"`
	} `yaml:"logging"`
}

func parseConfigFile(filename string) (config *Config) {
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	var configFile ConfigFile

	err = yaml.Unmarshal(yamlFile, &configFile)
	if err != nil {
		panic(err)
	}

	config = &Config{}

	config.gameName = configFile.GameName
	config.gamePath = configFile.GamePath

	if configFile.SpRooms != "" {
		for _, str := range strings.Split(configFile.SpRooms, ",") {
			num, err := strconv.Atoi(str)
			if err != nil {
				continue
			}

			config.spRooms = append(config.spRooms, num)
		}
	}

	config.badSounds = make(map[string]bool)
	if configFile.BadSounds != "" {
		for _, name := range strings.Split(configFile.BadSounds, ",") {
			config.badSounds[name] = true
		}
	}

	config.pictures = make(map[string]bool)
	if configFile.PictureNames != "" {
		for _, name := range strings.Split(configFile.PictureNames, ",") {
			config.pictures[name] = true
		}
	}

	if configFile.PicturePrefixes != "" {
		config.picturePrefixes = strings.Split(strings.ToLower(configFile.PicturePrefixes), ",")
	}

	config.battleAnimIds = make(map[int]bool)
	if configFile.BattleAnimIds != "" {
		for _, id := range strings.Split(configFile.BattleAnimIds, ",") {
			idInt, errconv := strconv.Atoi(id)
			if errconv != nil {
				continue
			}

			config.battleAnimIds[idInt] = true
		}
	}

	config.ipHubKey = configFile.IpHubKey

	if configFile.Logging.MaxSize != 0 {
		config.logging.maxSize = configFile.Logging.MaxSize
	} else {
		config.logging.maxSize = 50 // MB
	}
	if configFile.Logging.MaxBackups != 0 {
		config.logging.maxBackups = configFile.Logging.MaxBackups
	} else {
		config.logging.maxBackups = 6
	}
	if configFile.Logging.MaxAge != 0 {
		config.logging.maxAge = configFile.Logging.MaxAge
	} else {
		config.logging.maxAge = 28 // Days
	}

	return config
}
