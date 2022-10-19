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
	SpRooms         []int
	BadSounds       []string
	PictureNames    map[string]bool
	PicturePrefixes []string

	GameName string

	SignKey  []byte
	IPHubKey string

	Logging struct {
		File       string
		MaxSize    int
		MaxBackups int
		MaxAge     int
	}
}

type configFile struct {
	spRooms         string `yaml:"sp_rooms"`
	badSounds       string `yaml:"bad_sounds"`
	pictureNames    string `yaml:"picture_names"`
	picturePrefixes string `yaml:"picture_prefixes"`

	gameName string `yaml:"game_name"`

	signKey  string `yaml:"sign_key"`
	ipHubKey string `yaml:"iphub_key"`

	logging struct {
		file       string `yaml:"file"`
		maxSize    int    `yaml:"max_size"`
		maxBackups int    `yaml:"max_backups"`
		maxAge     int    `yaml:"max_age"`
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

	if configFile.spRooms != "" {
		for _, str := range strings.Split(configFile.spRooms, ",") {
			num, err := strconv.Atoi(str)
			if err != nil {
				return nil
			}

			config.SpRooms = append(config.SpRooms, num)
		}
	}

	if configFile.badSounds != "" {
		config.BadSounds = strings.Split(configFile.badSounds, ",")
	}

	config.PictureNames = make(map[string]bool)
	if configFile.pictureNames != "" {
		for _, name := range strings.Split(configFile.pictureNames, ",") {
			config.PictureNames[name] = true
		}
	}

	if configFile.picturePrefixes != "" {
		config.PicturePrefixes = strings.Split(configFile.picturePrefixes, ",")
	}

	config.GameName = configFile.gameName

	config.SignKey = []byte(configFile.signKey)
	config.IPHubKey = configFile.ipHubKey

	if configFile.logging.file == "" {
		config.Logging.File = "server.log"
	}
	if configFile.logging.maxSize == 0 {
		config.Logging.MaxSize = 50 // MB
	}
	if configFile.logging.maxBackups == 0 {
		config.Logging.MaxBackups = 6
	}
	if configFile.logging.maxAge == 0 {
		config.Logging.MaxAge = 28 // Days
	}

	fmt.Printf("%+v\n", config)

	return config
}
