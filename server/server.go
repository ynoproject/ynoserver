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
	"encoding/json"
	"flag"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	scheduler = gocron.NewScheduler(time.UTC)
)

func Start() {
	configFile := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	configFileData := parseConfig(*configFile)

	config = Config{
		gameName:     configFileData.GameName,

		signKey:  []byte(configFileData.SignKey),
		ipHubKey: configFileData.IPHubKey,
	}

	config.spriteNames = getCharSetList()
	config.systemNames = getSystemList()
	config.soundNames = getSoundList()

	// list of sound names to ignore
	if configFileData.BadSounds != "" {
		config.ignoredSoundNames = strings.Split(configFileData.BadSounds, ",")
	}

	// list of picture names to allow
	config.pictureNames = make(map[string]bool)
	if configFileData.PictureNames != "" {
		for _, name := range strings.Split(configFileData.PictureNames, ",") {
			config.pictureNames[name] = true
		}
	}

	// list of picture prefixes to allow
	if configFileData.PicturePrefixes != "" {
		config.picturePrefixes = strings.Split(configFileData.PicturePrefixes, ",")
	}

	setConditions()
	setBadges()
	setEventVms()

	globalConditions = getGlobalConditions()

	createRooms(getMapList(), atoiArray(strings.Split(configFileData.SpRooms, ",")))

	log.SetOutput(&lumberjack.Logger{
		Filename:   configFileData.Logging.File,
		MaxSize:    configFileData.Logging.MaxSize,
		MaxBackups: configFileData.Logging.MaxBackups,
		MaxAge:     configFileData.Logging.MaxAge,
	})
	log.SetFlags(log.Ldate | log.Ltime)

	initApi()
	initEvents()
	initBadges()
	initRankings()
	initSession()

	scheduler.StartAsync()

	http.HandleFunc("/room", handleRoom)
	http.HandleFunc("/session", handleSession)

	http.Serve(getListener(), nil)
}

func getListener() net.Listener {
	// remove socket file
	os.Remove("sockets/" + config.gameName + ".sock")

	// create unix socket at sockets/<game>.sock
	listener, err := net.Listen("unix", "sockets/"+config.gameName+".sock")
	if err != nil {
		log.Fatal(err)
	}

	// listen for connections to socket
	if err := os.Chmod("sockets/"+config.gameName+".sock", 0666); err != nil {
		log.Fatal(err)
	}

	return listener
}

func atoiArray(strArray []string) (intArray []int) {
	for _, str := range strArray {
		num, err := strconv.Atoi(str)
		if err != nil {
			return nil
		}

		intArray = append(intArray, num)
	}

	return intArray
}

func contains(s []int, num int) bool {
	for _, v := range s {
		if v == num {
			return true
		}
	}

	return false
}

func getIp(r *http.Request) string {
	return r.Header.Get("x-forwarded-for")
}

func isVpn(ip string) (vpn bool) {
	if config.ipHubKey == "" {
		return false // VPN checking is not available
	}

	req, err := http.NewRequest("GET", "https://v2.api.iphub.info/ip/"+ip, nil)
	if err != nil {
		return false
	}

	req.Header.Set("X-Key", config.ipHubKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var response interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}

	if response.(map[string]interface{})["block"].(float64) != 0 {
		vpn = true
	}

	return vpn
}

func isOkString(str string) bool {
	return regexp.MustCompile("^[A-Za-z0-9]+$").MatchString(str)
}

func writeLog(ip string, location string, payload string, errorcode int) {
	log.Printf("%v %v \"%v\" %v\n", ip, location, strings.Replace(payload, "\"", "'", -1), errorcode)
}

func writeErrLog(ip string, location string, payload string) {
	writeLog(ip, location, payload, 400)
}

func randString(length int) string {
	const runes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	const lenRunes = len(runes)

	b := make([]byte, length)
	for i := range b {
		b[i] = runes[rand.Intn(lenRunes)]
	}

	return string(b)
}

func buildMsg(segments []any) (message []byte) {
	for idx, segment := range segments {
		switch segment := segment.(type) {
		case byte:
			message = append(message, segment)
		case []byte:
			message = append(message, segment...)
		case string:
			message = append(message, []byte(segment)...)
		case []string:
			for strIdx, str := range segment {
				message = append(message, []byte(str)...)

				if strIdx != len(segment)-1 {
					message = append(message, delimBytes...)
				}
			}
		case int:
			message = append(message, []byte(strconv.Itoa(segment))...)
		case []int:
			for numIdx, num := range segment {
				message = append(message, []byte(strconv.Itoa(num))...)

				if numIdx != len(segment)-1 {
					message = append(message, delimBytes...)
				}
			}
		case bool:
			boolStr := "0"
			if segment {
				boolStr = "1"
			}

			message = append(message, []byte(boolStr)[0])
		default:
			continue
		}

		if idx != len(segments)-1 {
			message = append(message, delimBytes...)
		}
	}

	return message
}
