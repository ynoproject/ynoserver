package main

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
	IP              string `yaml:"ip"`
	Port            int    `yaml:"port"`
	SpRooms         string `yaml:"sp_rooms"`
	BadSounds       string `yaml:"bad_sounds"`
	PictureNames    string `yaml:"picture_names"`
	PicturePrefixes string `yaml:"picture_prefixes"`
	GameName        string `yaml:"game_name"`
	SignKey         string `yaml:"sign_key"`
	IPHubKey        string `yaml:"iphub_key"`
	Database        struct {
		User string `yaml:"user"`
		Pass string `yaml:"pass"`
		Host string `yaml:"host"`
		Name string `yaml:"name"`
	} `yaml:"database"`
	Logging struct {
		File       string `yaml:"file"`
		MaxSize    int    `yaml:"max_size"`
		MaxBackups int    `yaml:"max_backups"`
		MaxAge     int    `yaml:"max_age"`
	} `yaml:"logging"`
}

func parseConfig(file string) ConfigFile {
	yamlFile, err := os.ReadFile(file)
	if err != nil {
		panic(err)
	}

	var config ConfigFile
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}

	if config.IP == "" {
		config.IP = "127.0.0.1"
	}
	if config.Port == 0 {
		config.Port = 8080
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
