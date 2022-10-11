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
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-co-op/gocron"
)

var (
	sessionClients sync.Map
	session        = &Session{
		processMsgCh: make(chan *SessionMessage, 16),
		connect:      make(chan *ConnInfo, 4),
		unregister:   make(chan *SessionClient, 4),
	}
)

type Session struct {
	// Inbound messages from the clients.
	processMsgCh chan *SessionMessage

	// Connection requests from the clients.
	connect chan *ConnInfo

	// Unregister requests from clients.
	unregister chan *SessionClient
}

func initSession() {
	go session.run()

	s := gocron.NewScheduler(time.UTC)

	s.Every(5).Seconds().Do(func() {
		session.broadcast("pc", getSessionClientsLen())
		sendPartyUpdate()
	})

	s.StartAsync()
}

func (s *Session) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-Websocket-Protocol": {r.Header.Get("Sec-Websocket-Protocol")}})
	if err != nil {
		log.Println(err)
		return
	}

	var playerToken string
	token, ok := r.URL.Query()["token"]
	if ok && len(token[0]) == 32 {
		playerToken = token[0]
	}

	s.connect <- &ConnInfo{Connect: conn, Ip: getIp(r), Token: playerToken}
}

func (s *Session) run() {
	http.HandleFunc("/session", s.serve)
	for {
		select {
		case conn := <-s.connect:
			client := &SessionClient{
				session: s,
				conn:    conn.Connect,
				ip:      conn.Ip,
				send:    make(chan []byte, 16),
			}

			var banned bool
			if conn.Token != "" {
				client.uuid, client.name, client.rank, client.badge, banned, client.muted = getPlayerDataFromToken(conn.Token)
			}

			if client.uuid != "" {
				client.account = true
			} else {
				client.uuid, banned, client.muted = getOrCreatePlayerData(conn.Ip)
			}

			if banned || isIpBanned(conn.Ip) {
				writeErrLog(conn.Ip, "session", "player is banned")
				continue
			}

			if _, ok := sessionClients.Load(client.uuid); ok {
				writeErrLog(conn.Ip, "session", "session already exists for uuid")
				continue
			}

			var sameIp int
			sessionClients.Range(func(_, v any) bool {
				otherClient := v.(*SessionClient)

				if otherClient.ip == conn.Ip {
					sameIp++
				}

				return true
			})
			if sameIp >= 3 {
				writeErrLog(conn.Ip, "session", "too many connections from ip")
				continue
			}

			if client.badge == "" {
				client.badge = "null"
			}

			client.spriteName, client.spriteIndex, client.systemName = getPlayerGameData(client.uuid)

			go client.writePump()
			go client.readPump()

			client.sendMsg("s", client.uuid, client.rank, client.account, client.badge)

			// register client in the structures
			sessionClients.Store(client.uuid, client)

			writeLog(conn.Ip, "session", "connect", 200)
		case client := <-s.unregister:
			client.disconnected = true

			sessionClients.Delete(client.uuid)

			close(client.send)

			updatePlayerGameData(client)

			writeLog(client.ip, "session", "disconnect", 200)
		case message := <-s.processMsgCh:
			if errs := s.processMsgs(message); len(errs) > 0 {
				for _, err := range errs {
					writeErrLog(message.sender.ip, "session", err.Error())
				}
			}
		}
	}
}

func (s *Session) broadcast(segments ...any) {
	sessionClients.Range(func(_, v any) bool {
		client := v.(*SessionClient)

		client.sendMsg(segments...)

		return true
	})
}

func (s *Session) processMsgs(msg *SessionMessage) []error {
	var errs []error

	if len(msg.data) > 4096 {
		return append(errs, errors.New("bad request size"))
	}

	for _, v := range msg.data {
		if v < 32 {
			return append(errs, errors.New("bad byte sequence"))
		}
	}

	if !utf8.Valid(msg.data) {
		return append(errs, errors.New("invalid UTF-8"))
	}

	// message processing
	for _, msgStr := range strings.Split(string(msg.data), mdelim) {
		if err := s.processMsg(msgStr, msg.sender); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (s *Session) processMsg(msgStr string, sender *SessionClient) error {
	err := errors.New(msgStr)
	msgFields := strings.Split(msgStr, delim)

	if len(msgFields) == 0 {
		return err
	}

	switch msgFields[0] {
	case "i": // player info
		err = s.handleI(sender)
	case "name": // nick set
		err = s.handleName(msgFields, sender)
	case "ploc": // previous location
		err = s.handlePloc(msgFields, sender)
	case "gsay": // global say
		err = s.handleGSay(msgFields, sender)
	case "psay": // party say
		err = s.handlePSay(msgFields, sender)
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
		err = errors.New("unknown message type")
	}

	if err != nil {
		return err
	}

	writeLog(sender.ip, "session", msgStr, 200)

	return nil
}

func getSessionClientsLen() int {
	var length int

	sessionClients.Range(func(_, _ any) bool {
		length++

		return true
	})

	return length
}
