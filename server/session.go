/*
	Copyright (C) 2021-2024  The YNOproject Developers

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
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/fasthttp/websocket"
)

var (
	clients = NewSCMap()
)

func initSession() {
	logInitTask("session")

	// we need a sender
	var sender SessionClient

	var lastSentPlayerCount int
	scheduler.Every(5).Seconds().Do(func() {
		count := clients.GetAmount()

		if count != lastSentPlayerCount {
			sender.broadcast(buildMsg("pc", count))

			lastSentPlayerCount = count
		}
	})

	scheduler.Every(10).Seconds().Do(func() {
		sendPartyUpdate()
		sendFriendsUpdate()
	})

	scheduler.Cron("0 2,8,14,20 * * *").Do(func() {
		writeGamePlayerCount(clients.GetAmount())
	})

	go func() {
		stop := make(chan os.Signal, 1)

		signal.Notify(stop, syscall.SIGTERM)

		<-stop

		sender.broadcast(buildMsg("p", "0000000000000000", "YNO", "", 2, true, "null", [5]int{}))
		sender.broadcast(buildMsg("gsay", "0000000000000000", "0000", "0000", "0", 0, 0, "**The server is restarting.**", randString(12)))

		time.Sleep(time.Second)

		os.Exit(0)
	}()
}

func handleSession(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-Websocket-Protocol": {r.Header.Get("Sec-Websocket-Protocol")}})
	if err != nil {
		log.Println(err)
		return
	}

	joinSessionWs(conn, getIp(r), r.URL.Query().Get("token"))
}

func joinSessionWs(conn *websocket.Conn, ip string, token string) {
	c := &SessionClient{
		conn:          conn,
		ip:            ip,
		outbox:        make(chan []byte, 8),
		onlineFriends: make(map[string]bool),
		blockedUsers:  make(map[string]bool),
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())

	var banned bool
	if token != "" {
		c.uuid, c.name, c.rank, c.badge, banned, c.muted = getPlayerDataFromToken(token)
		if c.uuid != "" {
			c.medals = getPlayerMedals(c.uuid)
		}
	}

	if c.uuid != "" {
		c.account = true
	} else {
		c.uuid, banned, c.muted = getOrCreatePlayerData(ip)
	}

	if banned {
		writeErrLog(c.uuid, "sess", "player is banned")
		return
	}

	c.cacheParty() // don't log error because player is probably not in a party

	if client, ok := clients.Load(c.uuid); ok {
		client.cancel()
	}

	var sameIp int
	for _, client := range clients.Get() {
		if client.ip == ip {
			sameIp++
		}
	}
	if sameIp > 3 {
		writeErrLog(c.uuid, "sess", "too many connections from ip")
		return
	}

	if c.badge == "" {
		c.badge = "null"
	}

	for i := 0; i < 0xFFFF; i++ {
		var used bool
		for _, client := range clients.Get() {
			if client.id == i {
				used = true
			}
		}

		if !used {
			c.id = i
			break
		}
	}

	c.sprite, c.spriteIndex, c.system = getPlayerGameData(c.uuid)

	go c.msgWriter()

	// register client to the clients list
	clients.Store(c.uuid, c)

	go c.msgReader()

	err := c.addOrUpdatePlayerGameData()
	if err != nil {
		writeErrLog(c.uuid, "sess", err.Error())
	}

	writeLog(c.uuid, "sess", "connect", 200)
}

func (c *SessionClient) broadcast(msg []byte) {
	for _, client := range clients.Get() {
		select {
		case client.outbox <- buildMsg(msg):
		default:
			writeErrLog(c.uuid, "sess", "send channel is full")
		}
	}
}

func (c *SessionClient) processMsg(msg []byte) (err error) {
	if !utf8.Valid(msg) {
		return errors.New("invalid utf8")
	}

	var updateGameActivity bool

	switch msgFields := strings.Split(string(msg), delim); msgFields[0] {
	case "i": // player info
		err = c.handleI()
	case "name": // nick set
		err = c.handleName(msgFields)
	case "ploc": // previous location
		err = c.handlePloc(msgFields)
	case "lcol": // location colors
		err = c.handleLcol(msgFields)
	case "say": // map say
		err = c.handleSay(msgFields)
		updateGameActivity = true
	case "gsay", "psay": // global say and party say
		err = c.handleGPSay(msgFields)
		updateGameActivity = true
	case "l": // enter location(s)
		err = c.handleL(msgFields)
		updateGameActivity = true
	case "nl": // next expedition location(s)
		err = c.handleNl(msgFields)
	case "lp": // location player counts
		err = c.handleLp()
	case "pf": // friend list update
		err = c.handlePf()
	case "pt": // party update
		err = c.handlePt()
		if err != nil {
			c.outbox <- buildMsg("pt", "null")
		}
	case "ep": // event period
		err = c.handleEp()
	case "e": // event list
		err = c.handleE()
	case "eexp": // update expedition points
		err = c.handleEexp()
	case "eec": // claim expedition
		err = c.handleEec(msgFields)
	case "psi": // player screenshot info
		err = c.handlePsi(c.uuid, msgFields)
	case "pr": // private mode
		err = c.handlePr(msgFields)
		updateGameActivity = true
	case "hl": // hide location
		err = c.handleHl(msgFields)
		updateGameActivity = true
	default:
		err = errors.New("unknown message type")
	}
	if err != nil {
		return err
	}

	if updateGameActivity {
		err = c.updatePlayerGameActivity(true)
		if err != nil {
			writeErrLog(c.uuid, "sess", err.Error())
		}
	}

	writeLog(c.uuid, "sess", string(msg), 200)

	return nil
}
