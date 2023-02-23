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
	"crypto/rand"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/gorilla/websocket"
	"github.com/ynoproject/ynoserver/server/assets"
	"github.com/ynoproject/ynoserver/server/config"
	"github.com/ynoproject/ynoserver/server/security"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	hostGameId = "2kki"

	delim  = "\uffff"
	mdelim = "\ufffe"
)

var (
	scheduler = gocron.NewScheduler(time.UTC)

	serverConfig   *config.Config
	serverSecurity *security.Security
	gameAssets     *assets.Assets

	isHostServer bool

	delimBytes = []byte("\uffff")

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func Start() {
	configFile := flag.String("config", "config.yml", "Path to the configuration file")
	flag.Parse()

	serverConfig = config.ParseConfigFile(*configFile)

	isHostServer = serverConfig.GameName == hostGameId

	serverSecurity = security.New(serverConfig.SignKey)
	gameAssets = assets.GetAssets(serverConfig.GamePath)

	gameAssets.IgnoredSoundNames = serverConfig.BadSounds
	gameAssets.PictureNames = serverConfig.PictureNames
	gameAssets.PicturePrefixes = serverConfig.PicturePrefixes
	gameAssets.BattleAnimIds = serverConfig.BattleAnimIds

	setConditions()
	setBadges()
	setEventVms()

	globalConditions = getGlobalConditions()

	createRooms(gameAssets.MapIds, serverConfig.SpRooms)

	log.SetOutput(&lumberjack.Logger{
		Filename:   "logs/" + serverConfig.GameName + "/ynoserver.log",
		MaxSize:    serverConfig.Logging.MaxSize,
		MaxBackups: serverConfig.Logging.MaxBackups,
		MaxAge:     serverConfig.Logging.MaxAge,
	})
	log.SetFlags(log.Ldate | log.Ltime)

	initApi()
	initHistory()
	initEvents()
	initBadges()
	initSession()

	scheduler.StartAsync()

	http.HandleFunc("/room", handleRoom)
	http.HandleFunc("/session", handleSession)

	http.Serve(getListener(), nil)
}

func getListener() net.Listener {
	// remove socket file
	os.Remove("sockets/" + serverConfig.GameName + ".sock")

	// create unix socket at sockets/<game>.sock
	listener, err := net.Listen("unix", "sockets/"+serverConfig.GameName+".sock")
	if err != nil {
		log.Fatal(err)
	}

	// set socket file permissions
	if err := os.Chmod("sockets/"+serverConfig.GameName+".sock", 0666); err != nil {
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
	if serverConfig.IpHubKey == "" {
		return false // VPN checking is not available
	}

	req, err := http.NewRequest("GET", "https://v2.api.iphub.info/ip/"+ip, nil)
	if err != nil {
		return false
	}

	req.Header.Set("X-Key", serverConfig.IpHubKey)
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

func isOkString(str string) bool {
	return regexp.MustCompile("^[A-Za-z0-9]+$").MatchString(str)
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

				if strIdx+1 != len(segment) {
					message = append(message, delimBytes...)
				}
			}
		case map[string]bool:
			var strIdx int
			for str := range segment {
				message = append(message, []byte(str)...)

				if strIdx++; strIdx != len(segment) {
					message = append(message, delimBytes...)
				}
			}
		case int:
			message = append(message, []byte(strconv.Itoa(segment))...)
		case []int:
			for numIdx, num := range segment {
				message = append(message, []byte(strconv.Itoa(num))...)

				if numIdx+1 != len(segment) {
					message = append(message, delimBytes...)
				}
			}
		case map[int]bool:
			var numIdx int
			for num := range segment {
				message = append(message, []byte(strconv.Itoa(num))...)

				if numIdx++; numIdx != len(segment) {
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
