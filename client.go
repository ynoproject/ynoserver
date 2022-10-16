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

// RoomClient is a middleman between the websocket connection and the room.
type RoomClient struct {
	room     *Room
	sClient *SessionClient

	conn *websocket.Conn

	dcOnce sync.Once

	send    chan []byte
	receive chan []byte

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
	hClient *RoomClient

	conn *websocket.Conn
	ip   string

	dcOnce sync.Once

	send    chan []byte
	receive chan []byte

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

func (c *RoomClient) msgReader() {
	defer c.disconnect()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			writeLog(c.sClient.ip, strconv.Itoa(c.room.roomId), err.Error(), 500)
			break
		}

		c.receive <- message
	}
}

func (s *SessionClient) msgReader() {
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

		s.receive <- message
	}
}

func (c *RoomClient) msgWriter() {
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

func (s *SessionClient) msgWriter() {
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

func (c *RoomClient) msgProcessor() {
	for {
		message, ok := <-c.receive
		if !ok {
			return
		}

		if errs := c.room.processMsgs(c, message); len(errs) > 0 {
			for _, err := range errs {
				writeErrLog(c.sClient.ip, strconv.Itoa(c.room.roomId), err.Error())
			}
		}
	}
}

func (s *SessionClient) msgProcessor() {
	for {
		message, ok := <-s.receive
		if !ok {
			return
		}

		if errs := session.processMsgs(s, message); len(errs) > 0 {
			for _, err := range errs {
				writeErrLog(s.ip, "session", err.Error())
			}
		}
	}
}

func (c *RoomClient) sendMsg(segments ...any) {
	c.send <- buildMsg(segments)
}

func (s *SessionClient) sendMsg(segments ...any) {
	s.send <- buildMsg(segments)
}

func (c *RoomClient) disconnect() {
	c.dcOnce.Do(func() {
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		c.conn.WriteMessage(websocket.CloseMessage, nil)

		c.conn.Close()

		c.sClient.hClient = nil

		c.room.clients.Delete(c)

		c.room.broadcast(c, "d", c.sClient.id) // user %id% has disconnected message

		close(c.receive)

		writeLog(c.sClient.ip, strconv.Itoa(c.room.roomId), "disconnect", 200)
	})
}

func (s *SessionClient) disconnect() {
	s.dcOnce.Do(func() {
		s.conn.SetWriteDeadline(time.Now().Add(writeWait))
		s.conn.WriteMessage(websocket.CloseMessage, nil)

		s.conn.Close()

		clients.Delete(s.uuid)

		updatePlayerGameData(s)

		close(s.receive)

		writeLog(s.ip, "session", "disconnect", 200)
	})
}
