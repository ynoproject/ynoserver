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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-co-op/gocron"
)

var (
	sessionClients sync.Map
	session        = &Session{
		processMsgCh: make(chan *SessionMessage),
		connect:      make(chan *ConnInfo),
		unregister:   make(chan *SessionClient),
	}
)

type Session struct {
	// Registered clients.
	clients sync.Map

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
		session.broadcast([]byte("pc" + delim + strconv.Itoa(getSessionClientsLen())))
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
			var uuid string
			var name string
			var rank int
			var badge string
			var banned bool
			var muted bool
			var account bool

			if conn.Token != "" {
				uuid, name, rank, badge, banned, muted = getPlayerDataFromToken(conn.Token)
				if uuid != "" { //if we got a uuid back then we're logged in
					account = true
				}
			}

			if !account {
				uuid, banned, muted = getOrCreatePlayerData(conn.Ip)
			}

			if banned || isIpBanned(conn.Ip) {
				writeErrLog(conn.Ip, "session", "player is banned")
				continue
			}

			if _, ok := sessionClients.Load(uuid); ok {
				writeErrLog(conn.Ip, "session", "session already exists for uuid")
				continue
			}

			var sameIp int
			s.clients.Range(func(k, _ any) bool {
				otherClient := k.(*SessionClient)

				if otherClient.ip == conn.Ip {
					sameIp++
				}

				return true
			})
			if sameIp >= 3 {
				writeErrLog(conn.Ip, "session", "too many connections from ip")
				continue
			}

			if badge == "" {
				badge = "null"
			}

			spriteName, spriteIndex, systemName := getPlayerGameData(uuid)

			client := &SessionClient{
				session:     s,
				conn:        conn.Connect,
				send:        make(chan []byte, 256),
				ip:          conn.Ip,
				account:     account,
				name:        name,
				uuid:        uuid,
				rank:        rank,
				badge:       badge,
				muted:       muted,
				spriteName:  spriteName,
				spriteIndex: spriteIndex,
				systemName:  systemName,
			}
			go client.writePump()
			go client.readPump()

			client.sendPacket([]byte("s" + delim + uuid + delim + strconv.Itoa(rank) + delim + btoa(account) + delim + badge))

			//register client in the structures
			s.clients.Store(client, nil)
			sessionClients.Store(uuid, client)

			writeLog(conn.Ip, "session", "connect", 200)
		case client := <-s.unregister:
			if _, ok := s.clients.Load(client); ok {
				client.disconnect()
				writeLog(client.ip, "session", "disconnect", 200)
				continue
			}

			writeErrLog(client.ip, "session", "attempted to unregister nil client")
		case message := <-s.processMsgCh:
			if errs := s.processMsgs(message); len(errs) > 0 {
				for _, err := range errs {
					writeErrLog(message.sender.ip, "session", err.Error())
				}
			}
		}
	}
}

func (s *Session) broadcast(data []byte) {
	s.clients.Range(func(k, _ any) bool {
		client := k.(*SessionClient)

		client.sendPacket(data)

		return true
	})
}

func (s *Session) processMsgs(msg *SessionMessage) []error {
	var errs []error

	if len(msg.data) > 4096 {
		errs = append(errs, errors.New("bad request size"))
		return errs
	}

	for _, v := range msg.data {
		if v < 32 {
			errs = append(errs, errors.New("bad byte sequence"))
			return errs
		}
	}

	if !utf8.Valid(msg.data) {
		errs = append(errs, errors.New("invalid UTF-8"))
		return errs
	}

	//message processing
	msgs := strings.Split(string(msg.data), mdelim)

	for _, msgStr := range msgs {
		err := s.processMsg(msgStr, msg.sender)
		if err != nil {
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
	case "i": //player info
		err = s.handleI(msgFields, sender)
	case "name": //nick set
		err = s.handleName(msgFields, sender)
	case "ploc": //previous location
		err = s.handlePloc(msgFields, sender)
	case "gsay": //global say
		err = s.handleGSay(msgFields, sender)
	case "psay": //party say
		err = s.handlePSay(msgFields, sender)
	case "pt": //party update
		err = s.handlePt(msgFields, sender)
		if err != nil {
			sender.sendPacket(([]byte("pt" + delim + "null")))
		}
	case "ep": //event period
		err = s.handleEp(msgFields, sender)
	case "e": //event list
		err = s.handleE(msgFields, sender)
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
	var len int

	sessionClients.Range(func(_, _ any) bool {
		len++

		return true
	})

	return len
}
