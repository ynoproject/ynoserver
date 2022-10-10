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

package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	configFile := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	configFileData := parseConfig(*configFile)

	// list of sound names to ignore
	var ignoredSoundNames []string
	if configFileData.BadSounds != "" {
		ignoredSoundNames = strings.Split(configFileData.BadSounds, ",")
	}

	// list of picture names to allow
	pictureNames := make(map[string]bool)
	if configFileData.PictureNames != "" {
		for _, name := range strings.Split(configFileData.PictureNames, ",") {
			pictureNames[name] = true
		}
	}

	// list of picture prefixes to allow
	var picturePrefixes []string
	if configFileData.PicturePrefixes != "" {
		picturePrefixes = strings.Split(configFileData.PicturePrefixes, ",")
	}

	config = Config{
		ignoredSoundNames: ignoredSoundNames,
		pictureNames:      pictureNames,
		picturePrefixes:   picturePrefixes,
		gameName:          configFileData.GameName,

		signKey:  []byte(configFileData.SignKey),
		ipHubKey: configFileData.IPHubKey,
	}

	config.spriteNames = getCharSetList()
	config.soundNames = getSoundList()
	config.systemNames = getSystemList()

	setDatabase(configFileData.Database.User, configFileData.Database.Pass, configFileData.Database.Host, configFileData.Database.Name)

	setConditions()
	setBadges()
	setEventVms()

	globalConditions = getGlobalConditions()
	createAllHubs(getMapList(), atoiArray(strings.Split(configFileData.SpRooms, ",")))

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

	log.Fatalf("%v %v \"%v\" %v", configFileData.IP, "server", http.ListenAndServe(configFileData.IP+":"+strconv.Itoa(configFileData.Port), nil), 500)
}

func getCharSetList() map[string]bool {
	files, err := os.ReadDir(config.gameName + "/CharSet")
	if err != nil {
		panic(err)
	}

	charSets := make(map[string]bool)
	for _, file := range files {
		charSets[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return charSets
}

func getSoundList() map[string]bool {
	files, err := os.ReadDir(config.gameName + "/Sound")
	if err != nil {
		panic(err)
	}

	sounds := make(map[string]bool)
	for _, file := range files {
		sounds[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return sounds
}

func getSystemList() map[string]bool {
	files, err := os.ReadDir(config.gameName + "/System")
	if err != nil {
		panic(err)
	}

	systems := make(map[string]bool)
	for _, file := range files {
		systems[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return systems
}

func getMapList() []int {
	files, err := os.ReadDir(config.gameName + "/")
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

func isValidSprite(name string) bool {
	if name == "" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	return config.spriteNames[name]
}

func isValidSystem(name string, ignoreSingleQuotes bool) bool {
	if ignoreSingleQuotes {
		name = strings.ReplaceAll(name, "'", "")
	}

	return config.systemNames[name]
}

func isValidSound(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	return config.soundNames[name]
}

func isValidPicName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	if config.pictureNames[name] {
		return true
	}

	for _, prefix := range config.picturePrefixes {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return true
		}
	}

	return false
}

func getIp(r *http.Request) string { // this breaks if you're using a revproxy that isn't on 127.0.0.1
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if forwardedIp := r.Header.Get("x-forwarded-for"); ip == "127.0.0.1" && forwardedIp != "" {
		return forwardedIp
	}

	return ip
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

func buildMsg(segments []any) []byte {
	var message []byte

	for idx, segment := range segments {
		switch segment := segment.(type) {
		case byte:
			message = append(message, segment)
		case []byte:
			message = append(message, segment...)
		case string:
			message = append(message, []byte(segment)...)
		case int:
			message = append(message, []byte(strconv.Itoa(segment))...)
		case bool:
			boolStr := "0"
			if segment {
				boolStr = "1"
			}

			message = append(message, []byte(boolStr)[0])
		default:
			continue
		}

		if idx != len(segments) {
			message = append(message, delimBytes...)
		}
	}

	return message
}
