/*
	Copyright (C) 2021-2023  The YNOproject Developers

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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/go-co-op/gocron"
	"github.com/ynoproject/ynoserver/server/security"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	mainGameId = "2kki"

	delim  = "\uffff"
	mdelim = "\ufffe"
)

var (
	scheduler = gocron.NewScheduler(time.UTC)

	config         *Config
	serverSecurity *security.Security
	assets         *Assets

	isMainServer bool

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	isOkString = regexp.MustCompile("^[A-Za-z0-9]+$").MatchString
)

func Start() {
	fmt.Println("Now starting YNOserver...")

	configFile := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	config = parseConfigFile(*configFile)

	fmt.Printf("Current game ID is \"%s\".\n", config.gameName)

	isMainServer = config.gameName == mainGameId

	key, err := os.ReadFile("key.bin")
	if err != nil {
		log.Fatalf("failed to read key file!")
	}

	serverSecurity = security.New(key)
	assets = getAssets(config.gamePath)

	assets.ignoredSounds = config.badSounds
	assets.allowedPictures = config.pictures
	assets.allowedPicturePrefixes = config.picturePrefixes
	assets.battleAnims = config.battleAnimIds

	setConditions()
	setBadges()
	setEventVms()

	globalConditions = getGlobalConditions()

	createRooms(assets.maps, config.spRooms)

	log.SetOutput(&lumberjack.Logger{
		Filename:   "logs/" + config.gameName + "/ynoserver.log",
		MaxSize:    config.logging.maxSize,
		MaxBackups: config.logging.maxBackups,
		MaxAge:     config.logging.maxAge,
	})
	log.SetFlags(log.Ldate | log.Ltime)

	initApi()
	initHistory()
	initEvents()
	initBadges()
	initScreenshots()
	initSession()

	scheduler.StartAsync()

	http.HandleFunc("/room", handleRoom)
	http.HandleFunc("/session", handleSession)

	fmt.Print("Now serving requests.\n")

	http.Serve(getListener(), nil)
}

func logInitTask(taskName string) {
	fmt.Print("Initializing " + taskName + "...\n")
}

func logUpdateTask(taskName string) {
	fmt.Print("Updating " + taskName + "...\n")
}

func logTaskComplete() {
	fmt.Print("Done.\n")
}

func getListener() net.Listener {
	// remove socket file
	os.Remove("sockets/" + config.gameName + ".sock")

	// create unix socket at sockets/<game>.sock
	listener, err := net.Listen("unix", "sockets/"+config.gameName+".sock")
	if err != nil {
		log.Fatal(err)
	}

	// set socket file permissions
	if err := os.Chmod("sockets/"+config.gameName+".sock", 0666); err != nil {
		log.Fatal(err)
	}

	return listener
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

type IpHubResponse struct {
	Block int `json:"block"`
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

	var response IpHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return false
	}

	if response.Block != 0 {
		vpn = true
	}

	return vpn
}

func writeLog(uuid string, location string, payload string, errorcode int) {
	log.Printf("%v %v \"%v\" %v\n", uuid, location, strings.Replace(payload, "\"", "'", -1), errorcode)
}

func writeErrLog(uuid string, location string, payload string) {
	writeLog(uuid, location, payload, 400)
}

const randRunes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
const lenRandRunes = len(randRunes)

func randString(length int) string {
	b := make([]byte, length)

	rand.Read(b)

	for i := range b {
		b[i] = randRunes[int(b[i])%lenRandRunes]
	}

	return string(b)
}

func buildMsg(segments ...any) (message []byte) {
	for i, segment := range segments {
		switch segment := segment.(type) {
		case byte:
			message = append(message, segment)
		case []byte:
			message = append(message, segment...)
		case string:
			message = append(message, []byte(segment)...)
		case []string:
			for i, str := range segment {
				message = append(message, []byte(str)...)

				if i+1 != len(segment) {
					message = append(message, []byte(delim)...)
				}
			}
		case map[string]bool:
			var i int
			for str := range segment {
				message = append(message, []byte(str)...)

				if i++; i != len(segment) {
					message = append(message, []byte(delim)...)
				}
			}
		case int:
			message = append(message, []byte(strconv.Itoa(segment))...)
		case []int:
			for i, num := range segment {
				message = append(message, []byte(strconv.Itoa(num))...)

				if i+1 != len(segment) {
					message = append(message, []byte(delim)...)
				}
			}
		case map[int]bool:
			var i int
			for num := range segment {
				message = append(message, []byte(strconv.Itoa(num))...)

				if i++; i != len(segment) {
					message = append(message, []byte(delim)...)
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

		if i != len(segments)-1 {
			message = append(message, []byte(delim)...)
		}
	}

	return message
}

func getNanoId() string {
	timestamp := time.Now().UTC().UnixNano()

	return hex.EncodeToString([]byte{
		byte(timestamp >> 56),
		byte(timestamp >> 48),
		byte(timestamp >> 40),
		byte(timestamp >> 32),
		byte(timestamp >> 24),
		byte(timestamp >> 16),
		byte(timestamp >> 8),
		byte(timestamp),
	})
}
