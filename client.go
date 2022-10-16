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
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 4096
)

type Picture struct {
	name string

	positionX, positionY int
	mapX, mapY           int
	panX, panY           int

	magnify, topTrans, bottomTrans int

	red, green, blue, saturation int

	effectMode, effectPower int

	useTransparentColor, fixedToMap bool
}

// HubClient is a middleman between the websocket connection and the hub.
type HubClient struct {
	hub     *Hub
	sClient *SessionClient

	conn *websocket.Conn

	dcOnce sync.Once

	send    chan []byte
	receive chan *HubMessage

	key, counter uint32

	valid bool

	x, y, facing, spd int

	flash          [5]int
	repeatingFlash bool

	hidden bool

	pictures map[int]*Picture

	mapId, prevMapId, prevLocations string

	tags []string

	syncCoords bool

	minigameScores []int

	switchCache map[int]bool
	varCache    map[int]int
}

type SessionClient struct {
	hClient *HubClient

	conn *websocket.Conn
	ip   string

	dcOnce sync.Once

	send chan []byte

	id int

	account bool
	name    string
	uuid    string
	rank    int
	badge   string

	muted bool

	spriteName  string
	spriteIndex int

	systemName string
}

type HubMessage struct {
	sender *HubClient
	data   []byte
}

type SessionMessage struct {
	sender *SessionClient
	data   []byte
}

// The readPump and writePump functions are based on functions from
// https://github.com/gorilla/websocket/blob/master/examples/chat/client.go

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *HubClient) readPump() {
	defer c.disconnect()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			writeLog(c.sClient.ip, strconv.Itoa(c.hub.roomId), err.Error(), 500)
			break
		}

		c.receive <- &HubMessage{sender: c, data: message}
	}
}

func (s *SessionClient) readPump() {
	defer s.disconnect()

	s.conn.SetReadLimit(maxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error { s.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			writeLog(s.ip, "session", err.Error(), 500)
			break
		}

		session.processMsgCh <- &SessionMessage{sender: s, data: message}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *HubClient) writePump() {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		c.disconnect()
	}()

	for {
		select {
		case message := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *SessionClient) writePump() {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		s.disconnect()
	}()

	for {
		select {
		case message := <-s.send:
			s.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := s.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			s.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if err := s.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *HubClient) disconnect() {
	c.dcOnce.Do(func() {
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		c.conn.WriteMessage(websocket.CloseMessage, nil)

		c.conn.Close()

		c.sClient.hClient = nil

		c.hub.clients.Delete(c)

		c.hub.broadcast(c, "d", c.sClient.id) // user %id% has disconnected message

		close(c.receive)

		writeLog(c.sClient.ip, strconv.Itoa(c.hub.roomId), "disconnect", 200)
	})
}

func (s *SessionClient) disconnect() {
	s.dcOnce.Do(func() {
		s.conn.SetWriteDeadline(time.Now().Add(writeWait))
		s.conn.WriteMessage(websocket.CloseMessage, nil)

		s.conn.Close()

		clients.Delete(s.uuid)

		updatePlayerGameData(s)

		writeLog(s.ip, "session", "disconnect", 200)
	})
}

func (c *HubClient) handleMsg() {
	for {
		message, ok := <-c.receive
		if !ok {
			return
		}

		if errs := c.hub.processMsgs(message); len(errs) > 0 {
			for _, err := range errs {
				writeErrLog(c.sClient.ip, strconv.Itoa(c.hub.roomId), err.Error())
			}
		}
	}
}

func (c *HubClient) sendMsg(segments ...any) {
	c.send <- buildMsg(segments)
}

func (s *SessionClient) sendMsg(segments ...any) {
	s.send <- buildMsg(segments)
}
