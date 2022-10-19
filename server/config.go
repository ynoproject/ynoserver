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
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

var (
	config Config
)

type Config struct {
	spriteNames       map[string]bool
	systemNames       map[string]bool
	soundNames        map[string]bool
	ignoredSoundNames []string
	pictureNames      map[string]bool
	picturePrefixes   []string

	gameName string

	signKey  []byte
	ipHubKey string
}

type ConfigFile struct {
	SpRooms         string `yaml:"sp_rooms"`
	BadSounds       string `yaml:"bad_sounds"`
	PictureNames    string `yaml:"picture_names"`
	PicturePrefixes string `yaml:"picture_prefixes"`
	GameName        string `yaml:"game_name"`
	SignKey         string `yaml:"sign_key"`
	IPHubKey        string `yaml:"iphub_key"`
	Logging         struct {
		File       string `yaml:"file"`
		MaxSize    int    `yaml:"max_size"`
		MaxBackups int    `yaml:"max_backups"`
		MaxAge     int    `yaml:"max_age"`
	} `yaml:"logging"`
}

func parseConfig(file string) (config ConfigFile) {
	yamlFile, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}

	if config.Logging.File == "" {
		config.Logging.File = "server.log"
	}
	if config.Logging.MaxSize == 0 {
		config.Logging.MaxSize = 50 // MB
	}
	if config.Logging.MaxBackups == 0 {
		config.Logging.MaxBackups = 6
	}
	if config.Logging.MaxAge == 0 {
		config.Logging.MaxAge = 28 // Days
	}

	fmt.Printf("%+v\n", config)

	return config
}
