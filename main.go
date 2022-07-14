package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
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

	spriteNames, soundNames, systemNames, mapIds := getCharSetList(), getSoundList(), getSystemList(), getMapList()

	//list of sound names to ignore
	var ignoredSoundNames []string
	if configFileData.BadSounds != "" {
		ignoredSoundNames = strings.Split(configFileData.BadSounds, ",")
	}

	//list of picture names to allow
	var pictureNames []string
	if configFileData.PictureNames != "" {
		pictureNames = strings.Split(configFileData.PictureNames, ",")
	}

	// list of picture prefixes to allow
	var picturePrefixes []string
	if configFileData.PicturePrefixes != "" {
		picturePrefixes = strings.Split(configFileData.PicturePrefixes, ",")
	}

	config = Config{
		spriteNames:       spriteNames,
		systemNames:       systemNames,
		soundNames:        soundNames,
		ignoredSoundNames: ignoredSoundNames,
		pictureNames:      pictureNames,
		picturePrefixes:   picturePrefixes,
		gameName:          configFileData.GameName,

		signKey:  []byte(configFileData.SignKey),
		ipHubKey: configFileData.IPHubKey,
	}

	setDatabase(configFileData.Database.User, configFileData.Database.Pass, configFileData.Database.Host, configFileData.Database.Name)
	setConditions()
	setBadges()
	setEventVms()

	globalConditions = getGlobalConditions()
	createAllHubs(mapIds, atoiArray(strings.Split(configFileData.SpRooms, ",")))

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

	log.Fatalf("%v %v \"%v\" %v", configFileData.IP, "server", http.ListenAndServe(":"+strconv.Itoa(configFileData.Port), nil), 500)
}

func getCharSetList() []string {
	files, err := os.ReadDir(config.gameName + "/CharSet")
	if err != nil {
		panic(err)
	}

	var charsets []string
	for _, file := range files {
		charsets = append(charsets, file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))])
	}

	return charsets
}

func getSoundList() []string {
	files, err := os.ReadDir(config.gameName + "/Sound")
	if err != nil {
		panic(err)
	}

	var sounds []string
	for _, file := range files {
		sounds = append(sounds, file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))])
	}

	return sounds
}

func getSystemList() []string {
	files, err := os.ReadDir(config.gameName + "/System")
	if err != nil {
		panic(err)
	}

	var systems []string
	for _, file := range files {
		systems = append(systems, file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))])
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

func btoa(b bool) string { //bool to ascii int
	if b {
		return "1"
	}

	return "0"
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

	for _, otherName := range config.spriteNames {
		if otherName == name {
			return true
		}
	}
	return false
}

func isValidSystem(name string, ignoreSingleQuotes bool) bool {
	if ignoreSingleQuotes {
		name = strings.ReplaceAll(name, "'", "")
	}
	for _, otherName := range config.systemNames {
		if ignoreSingleQuotes {
			otherName = strings.ReplaceAll(otherName, "'", "")
		}
		if otherName == name {
			return true
		}
	}
	return false
}

func isValidSound(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range config.soundNames {
		if otherName == name {
			for _, ignoredName := range config.ignoredSoundNames {
				if ignoredName == name {
					return false
				}
			}
			return true
		}
	}
	return false
}

func isValidPicName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range config.pictureNames {
		if otherName == name {
			return true
		}
	}

	nameLower := strings.ToLower(name)
	for _, prefix := range config.picturePrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return true
		}
	}

	return false
}

func getIp(r *http.Request) string { //this breaks if you're using a revproxy that isn't on 127.0.0.1
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "127.0.0.1" && r.Header.Get("x-forwarded-for") != "" {
		ip = r.Header.Get("x-forwarded-for")
	}

	return ip
}

type IpHubResponse struct {
	Block int `json:"block"`
}

func isVpn(ip string) (vpn bool) {
	if config.ipHubKey == "" {
		return false //VPN checking is not available
	}

	req, err := http.NewRequest("GET", "http://v2.api.iphub.info/ip/"+ip, nil)
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
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var response IpHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}

	if response.Block != 0 {
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
