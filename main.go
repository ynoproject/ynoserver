package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
)

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
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

	var roomNames []string
	for i := 0; i < configFileData.NumRooms; i++ {
		roomNames = append(roomNames, strconv.Itoa(i))
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

	setDatabase()
	setConditions()
	setBadges()

	spRooms := strings.Split(configFileData.SpRooms, ",")

	createAllHubs(roomNames, spRooms)

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

	log.Fatalf("%v %v \"%v\" %v", configFileData.IP, "server", http.ListenAndServe(":"+strconv.Itoa(configFileData.Port), nil), 500)
}

func writeLog(ip string, roomName string, payload string, errorcode int) {
	log.Printf("%v %v \"%v\" %v\n", ip, roomName, strings.Replace(payload, "\"", "'", -1), errorcode)
}

func writeErrLog(ip string, roomName string, payload string) {
	writeLog(ip, roomName, payload, 400)
}

func isValidSpriteName(name string) bool {
	if name == "" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range config.spriteNames {
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func isValidSystemName(name string, ignoreSingleQuotes bool) bool {
	if ignoreSingleQuotes {
		name = strings.ReplaceAll(name, "'", "")
	}
	for _, otherName := range config.systemNames {
		if ignoreSingleQuotes {
			otherName = strings.ReplaceAll(otherName, "'", "")
		}
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func isValidSoundName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range config.soundNames {
		if strings.EqualFold(otherName, name) {
			for _, ignoredName := range config.ignoredSoundNames {
				if strings.EqualFold(ignoredName, name) {
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

	nameLower := strings.ToLower(name)
	for _, otherName := range config.pictureNames {
		if otherName == nameLower {
			return true
		}
	}
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
	IP          string `json:"ip"`
	CountryCode string `json:"countryCode"`
	CountryName string `json:"countryName"`
	Asn         int    `json:"asn"`
	Isp         string `json:"isp"`
	Block       int    `json:"block"`
}

func isVpn(ip string) (bool, error) {
	apiKey := config.ipHubKey

	if apiKey == "" {
		return false, nil //VPN checking is not available
	}

	req, err := http.NewRequest("GET", "http://v2.api.iphub.info/ip/"+ip, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("X-Key", apiKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var response IpHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return false, err
	}

	var blockedIp bool
	if response.Block == 0 {
		blockedIp = false
	} else {
		blockedIp = true
	}

	if response.Block > 0 {
		log.Printf("Connection Blocked %v %v %v %v\n", response.IP, response.CountryName, response.Isp, response.Block)
		return false, errors.New("connection banned")
	}

	return blockedIp, nil
}

func globalBroadcast(inpData []byte) {
	for _, client := range allClients {
		client.send <- inpData
	}
}

func btoa(b bool) string { //bool to ascii int
	if b {
		return "1"
	}

	return "0"
}
