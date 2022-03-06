// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"net/http"
	"log"
	"strings"
	"regexp"
	"database/sql"
	"github.com/gorilla/websocket"
	_ "github.com/go-sql-driver/mysql"
)

var (
	maxID = 512
	totalPlayerCount = 0
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	isOkName = regexp.MustCompile("^[A-Za-z0-9]+$").MatchString
	paramDelimStr = "\uffff"
	msgDelimStr = "\ufffe"
)

type ConnInfo struct {
	Connect *websocket.Conn
	Ip string
}

type Config struct {
	spriteNames []string
	systemNames []string
	soundNames []string
	ignoredSoundNames []string
	pictureNames []string
	picturePrefixes []string

	gameName string

	signKey string
	ipHubKey string

	dbUser string
	dbPass string
	dbHost string
	dbName string
}

func writeLog(ip string, roomName string, payload string, errorcode int) {
	log.Printf("%v %v \"%v\" %v\n", ip, roomName, strings.Replace(payload, "\"", "'", -1), errorcode)
}

func writeErrLog(ip string, roomName string, payload string) {
	writeLog(ip, roomName, payload, 400)
}

func GetConfig(spriteNames []string, systemNames []string, soundNames []string, ignoredSoundNames []string, pictureNames []string, picturePrefixes []string, gameName string, signKey string, ipHubKey string, dbUser string, dbPass string, dbHost string, dbName string) (Config) {
	c := Config{
		spriteNames: spriteNames,
		systemNames: systemNames,
		soundNames: soundNames,
		ignoredSoundNames: ignoredSoundNames,
		pictureNames: pictureNames,
		picturePrefixes: picturePrefixes,
		gameName: gameName,

		signKey: signKey,
		ipHubKey: ipHubKey,

		dbUser: dbUser,
		dbPass: dbPass,
		dbHost: dbHost,
		dbName: dbName,
	}

	return c
}

func CreateAllHubs(roomNames []string, config Config) {
	h := HubController{
		config: config,
		database: getDatabaseHandle(config),
	}

	for _, roomName := range roomNames {
		h.addHub(roomName)
	}
}

func NewHub(roomName string, h *HubController) *Hub {
	return &Hub{
		processMsgCh:  make(chan *Message),
		connect:   make(chan *ConnInfo),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		id: make(map[int]bool),
		roomName: roomName,
		controller: h,
	}
}

type IpHubResponse struct {
	IP          string `json:"ip"`
	CountryCode string `json:"countryCode"`
	CountryName string `json:"countryName"`
	Asn         int    `json:"asn"`
	Isp         string `json:"isp"`
	Block       int    `json:"block"`
}

func GetPlayerCount() int {
	return totalPlayerCount
}

func getDatabaseHandle(c Config) (*sql.DB) {
	db, err := sql.Open("mysql", c.dbUser + ":" + c.dbPass + "@tcp(" + c.dbHost + ")/" + c.dbName)
	if err != nil {
		return nil
	}

	return db
}
