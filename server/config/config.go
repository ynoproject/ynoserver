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

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	GameName string
	GamePath string

	SpRooms         []int
	BadSounds       map[string]bool
	PictureNames    map[string]bool
	PicturePrefixes []string
	BattleAnimIds   map[int]bool

	SignKey  []byte
	IPHubKey string

	Logging struct {
		MaxSize    int
		MaxBackups int
		MaxAge     int
	}
}

type configFile struct {
	GameName string `yaml:"game_name"`
	GamePath string `yaml:"game_path"`

	SpRooms         string `yaml:"sp_rooms"`
	BadSounds       string `yaml:"bad_sounds"`
	PictureNames    string `yaml:"picture_names"`
	PicturePrefixes string `yaml:"picture_prefixes"`
	BattleAnimIds   string `yaml:"battle_anim_ids"`

	SignKey  string `yaml:"sign_key"`
	IPHubKey string `yaml:"iphub_key"`

	Logging struct {
		MaxSize    int `yaml:"max_size"`
		MaxBackups int `yaml:"max_backups"`
		MaxAge     int `yaml:"max_age"`
	} `yaml:"logging"`
}

func ParseConfigFile(filename string) (config *Config) {
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	var configFile configFile

	err = yaml.Unmarshal(yamlFile, &configFile)
	if err != nil {
		panic(err)
	}

	config = &Config{}

	config.GameName = configFile.GameName
	config.GamePath = configFile.GamePath

	if configFile.SpRooms != "" {
		for _, str := range strings.Split(configFile.SpRooms, ",") {
			num, err := strconv.Atoi(str)
			if err != nil {
				continue
			}

			config.SpRooms = append(config.SpRooms, num)
		}
	}

	config.BadSounds = make(map[string]bool)
	if configFile.BadSounds != "" {
		for _, name := range strings.Split(configFile.BadSounds, ",") {
			config.BadSounds[name] = true
		}
	}

	config.PictureNames = make(map[string]bool)
	if configFile.PictureNames != "" {
		for _, name := range strings.Split(configFile.PictureNames, ",") {
			config.PictureNames[name] = true
		}
	}

	if configFile.PicturePrefixes != "" {
		config.PicturePrefixes = strings.Split(configFile.PicturePrefixes, ",")
	}

	config.BattleAnimIds = make(map[int]bool)
	if configFile.BattleAnimIds != "" {
		for _, id := range strings.Split(configFile.BattleAnimIds, ",") {
			idInt, errconv := strconv.Atoi(id)
			if errconv != nil {
				continue
			}

			config.BattleAnimIds[idInt] = true
		}
	}

	config.SignKey = []byte(configFile.SignKey)
	config.IPHubKey = configFile.IPHubKey

	if configFile.Logging.MaxSize != 0 {
		config.Logging.MaxSize = configFile.Logging.MaxSize
	} else {
		config.Logging.MaxSize = 50 // MB
	}
	if configFile.Logging.MaxBackups != 0 {
		config.Logging.MaxBackups = configFile.Logging.MaxBackups
	} else {
		config.Logging.MaxBackups = 6
	}
	if configFile.Logging.MaxAge != 0 {
		config.Logging.MaxAge = configFile.Logging.MaxAge
	} else {
		config.Logging.MaxAge = 28 // Days
	}

	fmt.Printf("%+v\n", config)

	return config
}
