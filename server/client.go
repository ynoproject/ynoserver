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
	"fmt"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096

	maxPictures = 1000
)

type Picture struct {
	name string

	posX, posY int
	mapX, mapY int
	panX, panY int

	magnify, topTrans, bottomTrans int

	red, green, blue, saturation int

	effectMode, effectPower int

	useTransparentColor, fixedToMap bool

	spritesheetCols, spritesheetRows   int
	spritesheetFrame, spritesheetSpeed int
	spritesheetPlayOnce                bool

	mapLayer, battleLayer, flags, blendMode, origin int

	flipX, flipY bool
}

// SessionClient
type SessionClient struct {
	roomC *RoomClient

	conn *websocket.Conn
	ip   string

	ctx    context.Context
	cancel context.CancelFunc

	outbox chan []byte

	id int

	account bool
	name    string
	uuid    string
	rank    int
	badge   string
	medals  [5]int

	muted bool

	sprite      string
	spriteIndex int

	system string

	private      bool
	hideLocation bool
	partyId      int

	onlineFriends map[string]bool
	blockedUsers  map[string]bool
}

func (c *SessionClient) msgReader() {
	defer c.cancel()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				return
			}

			err = c.processMsg(message)
			if err != nil {
				writeErrLog(c.uuid, "sess", err.Error())
			}
		}
	}
}

func (c *SessionClient) msgWriter() {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()

		c.cancel()
		c.disconnect()
	}()

	for {
		select {
		case <-c.ctx.Done():
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1028, ""))

			return
		case message := <-c.outbox:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				return
			}
		}
	}
}

func (c *SessionClient) disconnect() {
	// unregister
	clients.Delete(c.uuid)

	// close conn, ends reader and processor
	c.conn.Close()

	err := c.updatePlayerGameActivity(false)
	if err != nil {
		writeErrLog(c.uuid, "sess", err.Error())
	}

	writeLog(c.uuid, "sess", "disconnect", 200)
}

// RoomClient
type RoomClient struct {
	room    *Room
	session *SessionClient

	conn *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc

	outbox chan []byte

	key, counter uint32

	x, y, facing, speed int

	flash          [5]int
	repeatingFlash bool
	transparency   int
	hidden         bool

	pictures [maxPictures]*Picture

	mapId, prevMapId, prevLocations string

	locations   []string
	locationIds []int

	tags []string

	syncCoords bool

	minigameScores []int

	switchCache map[int]bool
	varCache    map[int]int
}

func (c *RoomClient) msgReader() {
	defer c.cancel()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				return
			}

			errs := c.processMsgs(message)
			if len(errs) != 0 {
				for _, err := range errs {
					writeErrLog(c.session.uuid, c.mapId, err.Error())
				}
			}
		}
	}
}

func (c *RoomClient) msgWriter() {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()

		c.cancel()
		c.disconnect()
	}()

	for {
		select {
		case <-c.ctx.Done():
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1028, ""))

			return
		case message := <-c.outbox:
			for len(c.outbox) != 0 { // for each extra message in the channel
				if len(message) > maxMessageSize-256 { // stop if we're close to the message size limit
					break
				}

				message = append(message, []byte(mdelim)...) // add message delimiter
				message = append(message, <-c.outbox...)     // write next message contents
			}

			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.BinaryMessage, message)
			if err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				return
			}
		}
	}
}

func (c *RoomClient) disconnect() {
	c.cancel()

	// unregister
	c.leaveRoom()

	// close conn, ends reader and processor
	c.conn.Close()

	writeLog(c.session.uuid, c.mapId, "disconnect", 200)
}

func (c *RoomClient) reset() {
	c.x = -1
	c.y = -1
	c.facing = 0
	c.speed = 0

	c.flash = [5]int{}
	c.repeatingFlash = false

	c.hidden = false

	c.pictures = [maxPictures]*Picture{}

	c.mapId = fmt.Sprintf("%04d", c.room.id)
	c.prevMapId = ""
	c.prevLocations = ""

	c.locations = nil

	// don't clear tags

	c.syncCoords = false

	c.minigameScores = nil

	c.switchCache = make(map[int]bool)
	c.varCache = make(map[int]int)
}

type SClientMap struct {
	clients map[string]*SessionClient
	mutex   sync.RWMutex
}

func NewSCMap() *SClientMap {
	return &SClientMap{
		clients: make(map[string]*SessionClient),
	}
}

func (m *SClientMap) Store(uuid string, client *SessionClient) {
	m.mutex.Lock()

	m.clients[uuid] = client

	m.mutex.Unlock()
}

func (m *SClientMap) Load(uuid string) (*SessionClient, bool) {
	m.mutex.RLock()

	client, ok := m.clients[uuid]

	m.mutex.RUnlock()

	return client, ok
}

func (m *SClientMap) Delete(uuid string) {
	m.mutex.Lock()

	delete(m.clients, uuid)

	m.mutex.Unlock()
}

func (m *SClientMap) Get() []*SessionClient {
	m.mutex.RLock()

	clients := make([]*SessionClient, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}

	m.mutex.RUnlock()

	return clients
}

func (m *SClientMap) GetAmount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.clients)
}

func (m *SClientMap) Exists(uuid string) bool {
	m.mutex.RLock()

	_, ok := m.clients[uuid]

	m.mutex.RUnlock()

	return ok
}
