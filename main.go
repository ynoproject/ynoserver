package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

func contains(s []string, num int) bool {
	for _, v := range s {
		if v == strconv.Itoa(num) {
			return true
		}
	}

	return false
}

func main() {
	config_file := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	configFileData := ParseConfig(*config_file)

	res_index_data, err := ioutil.ReadFile(configFileData.IndexPath)
	if err != nil {
		log.Fatal(err)
	}

	var res_index interface{}

	err = json.Unmarshal(res_index_data, &res_index)
	if err != nil {
		log.Fatal(err)
	}

	//list of valid game character sprite resource keys
	var spriteNames []string
	for k := range res_index.(map[string]interface{})["cache"].(map[string]interface{})["charset"].(map[string]interface{}) {
		if k != "_dirname" {
			spriteNames = append(spriteNames, k)
		}
	}

	//list of valid sound resource keys
	var soundNames []string
	for k := range res_index.(map[string]interface{})["cache"].(map[string]interface{})["sound"].(map[string]interface{}) {
		if k != "_dirname" {
			soundNames = append(soundNames, k)
		}
	}

	//list of valid system resource keys
	var systemNames []string
	for k := range res_index.(map[string]interface{})["cache"].(map[string]interface{})["system"].(map[string]interface{}) {
		if k != "_dirname" {
			systemNames = append(systemNames, k)
		}
	}

	//list of invalid sound names
	var ignoredSoundNames []string
	if configFileData.BadSounds != "" {
		ignoredSoundNames = strings.Split(configFileData.BadSounds, ",")
	}

	//list of valid picture names
	var pictureNames []string
	if configFileData.PictureNames != "" {
		pictureNames = strings.Split(configFileData.PictureNames, ",")
	}

	// list of valid picture prefixes
	var picturePrefixes []string
	if configFileData.PicturePrefixes != "" {
		picturePrefixes = strings.Split(configFileData.PicturePrefixes, ",")
	}

	badRooms := strings.Split(configFileData.BadRooms, ",")

	var roomNames []string
	for i := 0; i < configFileData.NumRooms; i++ {
		if !contains(badRooms, i) {
			roomNames = append(roomNames, strconv.Itoa(i))
		}
	}

	config = Config{
		spriteNames:       spriteNames,
		systemNames:       systemNames,
		soundNames:        soundNames,
		ignoredSoundNames: ignoredSoundNames,
		pictureNames:      pictureNames,
		picturePrefixes:   picturePrefixes,
		gameName:          configFileData.GameName,

		signKey:  configFileData.SignKey,
		ipHubKey: configFileData.IPHubKey,

		dbUser: configFileData.Database.User,
		dbPass: configFileData.Database.Pass,
		dbHost: configFileData.Database.Host,
		dbName: configFileData.Database.Name,
	}

	SetConditions()
	SetBadges()

	CreateAllHubs(roomNames)

	log.SetOutput(&lumberjack.Logger{
		Filename:   configFileData.Logging.File,
		MaxSize:    configFileData.Logging.MaxSize,
		MaxBackups: configFileData.Logging.MaxBackups,
		MaxAge:     configFileData.Logging.MaxAge,
	})
	log.SetFlags(log.Ldate | log.Ltime)

	StartApi()
	StartEvents()
	StartRankings()

	log.Fatalf("%v %v \"%v\" %v", configFileData.IP, "server", http.ListenAndServe(":"+strconv.Itoa(configFileData.Port), nil), 500)
}
