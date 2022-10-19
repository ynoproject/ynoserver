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
	"log"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

var (
	clients sync.Map
	session = &Session{}
)

type Session struct {
	lastId int
}

func initSession() {
	scheduler.Every(5).Seconds().Do(func() {
		session.broadcast("pc", getSessionClientsLen())
		sendPartyUpdate()
	})
}

func handleSession(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-Websocket-Protocol": {r.Header.Get("Sec-Websocket-Protocol")}})
	if err != nil {
		log.Println(err)
		return
	}

	var playerToken string
	if token, ok := r.URL.Query()["token"]; ok && len(token[0]) == 32 {
		playerToken = token[0]
	}

	session.addClient(conn, getIp(r), playerToken)
}

func (s *Session) addClient(conn *websocket.Conn, ip string, token string) {
	client := &SessionClient{
		conn:      conn,
		ip:        ip,
		writerEnd: make(chan bool, 1),
		send:      make(chan []byte, 16),
		receive:   make(chan []byte, 16),
	}

	var banned bool
	if token != "" {
		client.uuid, client.name, client.rank, client.badge, banned, client.muted = getPlayerDataFromToken(token)
	}

	if client.uuid != "" {
		client.account = true
	} else {
		client.uuid, banned, client.muted = getOrCreatePlayerData(ip)
	}

	if banned {
		writeErrLog(ip, "session", "player is banned")
		return
	}

	if _, ok := clients.Load(client.uuid); ok {
		writeErrLog(ip, "session", "session already exists for uuid")
		return
	}

	var sameIp int
	clients.Range(func(_, v any) bool {
		if v.(*SessionClient).ip == ip {
			sameIp++
		}

		return true
	})
	if sameIp >= 3 {
		writeErrLog(ip, "session", "too many connections from ip")
		return
	}

	if client.badge == "" {
		client.badge = "null"
	}

	client.id = s.lastId
	s.lastId++

	client.spriteName, client.spriteIndex, client.systemName = getPlayerGameData(client.uuid)

	// register client to the clients list
	clients.Store(client.uuid, client)

	// queue s message
	client.sendMsg("s", client.uuid, client.rank, client.account, client.badge)

	go client.msgProcessor()

	go client.msgWriter()
	go client.msgReader()

	writeLog(ip, "session", "connect", 200)
}

func (s *Session) broadcast(segments ...any) {
	clients.Range(func(_, v any) bool {
		v.(*SessionClient).sendMsg(segments...)

		return true
	})
}

func (s *Session) processMsg(sender *SessionClient, msg []byte) (err error) {
	if !utf8.Valid(msg) {
		return errInvalidUTF8
	}

	msgFields := strings.Split(string(msg), delim)

	switch msgFields[0] {
	case "i": // player info
		err = s.handleI(sender)
	case "name": // nick set
		err = s.handleName(sender, msgFields)
	case "ploc": // previous location
		err = s.handlePloc(sender, msgFields)
	case "gsay": // global say
		err = s.handleGSay(sender, msgFields)
	case "psay": // party say
		err = s.handlePSay(sender, msgFields)
	case "pt": // party update
		err = s.handlePt(sender)
		if err != nil {
			sender.sendMsg("pt", "null")
		}
	case "ep": // event period
		err = s.handleEp(sender)
	case "e": // event list
		err = s.handleE(sender)
	default:
		err = errUnkMsgType
	}
	if err != nil {
		return err
	}

	writeLog(sender.ip, "session", string(msg), 200)

	return nil
}

func getSessionClientsLen() (length int) {
	clients.Range(func(_, _ any) bool {
		length++

		return true
	})

	return length
}
