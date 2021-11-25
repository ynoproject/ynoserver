package orbserver

import (
	"io/ioutil"
	"fmt"
	"gopkg.in/yaml.v2"
)

type ServerConfig struct {
	IP        string `yaml:"ip"`
	Port      int    `yaml:"port"`
	IndexPath string `yaml:"index_path"`
	NumRooms  int    `yaml:"num_rooms"`
	Logging   struct {
		File       string `yaml:"file"`
		MaxSize    int    `yaml:"max_size"`
		MaxBackups int    `yaml:"max_backups"`
		MaxAge     int    `yaml:"max_age"`
	} `yaml:"logging"`
}

func ParseConfig(file string) ServerConfig {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}

	var config ServerConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}

	if config.IndexPath == "" {
		config.IndexPath = "games/default/index.json"
	}
	if config.IP == "" {
		config.IP = "127.0.0.1"
	}
	if config.Port == 0 {
		config.Port = 8080
	}
	if config.NumRooms == 0 {
		config.NumRooms = 100
	}
	if config.Logging.File == "" {
		config.Logging.File = "orbs.log"
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
