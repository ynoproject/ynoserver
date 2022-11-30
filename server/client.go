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
	"fmt"
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

// SessionClient
type SessionClient struct {
	rClient *RoomClient

	conn *websocket.Conn
	ip   string

	dcOnce sync.Once

	writerEnd chan bool
	writerWg  sync.WaitGroup

	send, receive chan []byte

	id int

	account bool
	name    string
	uuid    string
	rank    int
	badge   string
	medals  [5]int

	muted bool

	spriteName  string
	spriteIndex int

	systemName string
}

func (s *SessionClient) msgReader() {
	s.conn.SetReadLimit(maxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(pongWait))
	s.conn.SetPongHandler(func(string) error { s.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			break
		}

		s.receive <- message
	}

	close(s.receive)

	s.disconnect()
}

func (s *SessionClient) msgWriter() {
	s.writerWg.Add(1)
	ticker := time.NewTicker(pingPeriod)

	var terminate bool
	for !terminate {
		select {
		case <-s.writerEnd:
			s.conn.SetWriteDeadline(time.Now().Add(writeWait))

			s.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1028, ""))

			terminate = true
		case message := <-s.send:
			s.conn.SetWriteDeadline(time.Now().Add(writeWait))

			err := s.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				terminate = true
			}
		case <-ticker.C:
			s.conn.SetWriteDeadline(time.Now().Add(writeWait))

			err := s.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				terminate = true
			}
		}
	}

	ticker.Stop()
	s.writerWg.Done()

	s.disconnect()
}

func (s *SessionClient) msgProcessor() {
	for {
		message, ok := <-s.receive
		if !ok {
			return
		}

		err := s.processMsg(message)
		if err != nil {
			writeErrLog(s.uuid, "sess", err.Error())
		}
	}
}

func (s *SessionClient) sendMsg(segments ...any) {
	s.send <- buildMsg(segments)
}

func (s *SessionClient) disconnect() {
	s.dcOnce.Do(func() {
		// unregister
		clients.Delete(s.uuid)

		// send terminate signal to writer
		close(s.writerEnd)

		// wait for writer to end
		s.writerWg.Wait()

		// close conn, ends reader and processor
		s.conn.Close()

		updatePlayerGameData(s)

		writeLog(s.uuid, "sess", "disconnect", 200)

		// disconnect rClient if connected
		if s.rClient != nil {
			s.rClient.disconnect()
		}
	})
}

// RoomClient
type RoomClient struct {
	room    *Room
	sClient *SessionClient

	conn *websocket.Conn

	dcOnce sync.Once

	writerEnd chan bool
	writerWg  sync.WaitGroup

	send, receive chan []byte

	key, counter uint32

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

func (c *RoomClient) msgReader() {
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		c.receive <- message
	}

	close(c.receive)

	c.disconnect()
}

func (c *RoomClient) msgWriter() {
	c.writerWg.Add(1)
	ticker := time.NewTicker(pingPeriod)

	var terminate bool
	for !terminate {
		select {
		case <-c.writerEnd:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1028, ""))

			terminate = true
		case message := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				terminate = true
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				terminate = true
			}
		}
	}

	ticker.Stop()
	c.writerWg.Done()

	c.disconnect()
}

func (c *RoomClient) msgProcessor() {
	for {
		message, ok := <-c.receive
		if !ok {
			return
		}

		errs := c.processMsgs(message)
		if len(errs) > 0 {
			for _, err := range errs {
				writeErrLog(c.sClient.uuid, c.mapId, err.Error())
			}
		}
	}
}

func (c *RoomClient) sendMsg(segments ...any) {
	c.send <- buildMsg(segments)
}

func (c *RoomClient) disconnect() {
	c.dcOnce.Do(func() {
		// unbind rClient from session
		c.sClient.rClient = nil

		// unregister
		c.leaveRoom()

		// send terminate signal to writer
		close(c.writerEnd)

		// wait for writer to end
		c.writerWg.Wait()

		// close conn, ends reader and processor
		c.conn.Close()

		writeLog(c.sClient.uuid, c.mapId, "disconnect", 200)
	})
}

func (c *RoomClient) reset() {
	c.x = 0
	c.y = 0
	c.facing = 0
	c.spd = 0

	c.flash = [5]int{}
	c.repeatingFlash = false

	c.hidden = false

	c.pictures = make(map[int]*Picture)

	c.mapId = fmt.Sprintf("%04d", c.room.id)
	c.prevMapId = ""
	c.prevLocations = ""

	// don't clear tags

	c.syncCoords = false

	c.minigameScores = []int{}

	c.switchCache = make(map[int]bool)
	c.varCache = make(map[int]int)
}
