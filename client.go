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

	positionX int
	positionY int
	mapX      int
	mapY      int
	panX      int
	panY      int

	magnify     int
	topTrans    int
	bottomTrans int

	red        int
	green      int
	blue       int
	saturation int

	effectMode  int
	effectPower int

	useTransparentColor bool
	fixedToMap          bool
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	session *SessionClient
	hub     *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	id int

	key     uint32
	counter uint32

	valid bool

	x, y   int
	facing int
	spd    int

	flash          [5]int
	repeatingFlash bool

	hidden bool

	pictures map[int]*Picture

	mapId         string
	prevMapId     string
	prevLocations string

	tags []string

	syncCoords bool

	minigameScores []int

	switchCache map[int]bool
	varCache    map[int]int
}

type SessionClient struct {
	session *Session

	conn *websocket.Conn
	send chan []byte

	ip string

	account bool
	name    string
	uuid    string
	rank    int
	badge   string

	bound bool

	muted bool

	spriteName  string
	spriteIndex int

	systemName string
}

type Message struct {
	data   []byte
	sender *Client //who sent the message
}

type SessionMessage struct {
	data   []byte
	sender *SessionClient //who sent the message
}

// The readPump and writePump functions are based on functions from
// https://github.com/gorilla/websocket/blob/master/examples/chat/client.go

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				writeLog(c.session.ip, strconv.Itoa(c.hub.roomId), err.Error(), 500)
			}
			break
		}
		c.hub.processMsgCh <- &Message{data: message, sender: c}
	}
}

func (c *SessionClient) readPump() {
	defer func() {
		session.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				writeLog(c.ip, "session", err.Error(), 500)
			}
			break
		}
		session.processMsgCh <- &SessionMessage{data: message, sender: c}
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
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

func (c *SessionClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
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

func (c *Client) disconnect() {
	c.session.bound = false

	close(c.send)

	c.hub.id.Delete(c.id)
	c.hub.clients.Delete(c)
	hubClients.Delete(c.session.uuid)
	c.hub.broadcast([]byte("d" + delim + strconv.Itoa(c.id))) //user %id% has disconnected message
}

func (s *SessionClient) disconnect() {
	close(s.send)

	updatePlayerGameData(s) //update database

	s.session.clients.Delete(s)
	sessionClients.Delete(s.uuid)
}

func (c *Client) sendPacket(data []byte) {
	defer recover()

	select {
	case c.send <- data:
	default:
		c.disconnect()
	}
}

func (s *SessionClient) sendPacket(data []byte) {
	defer recover()

	select {
	case s.send <- data:
	default:
		s.disconnect()
	}
}
