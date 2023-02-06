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
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

var rooms = make(map[int]*Room)

type Room struct {
	id           int
	singleplayer bool
	
	clients []*RoomClient

	conditions []*Condition
	minigames  []*Minigame
}

func createRooms(roomIds []int, spRooms []int) {
	for _, roomId := range roomIds {
		rooms[roomId] = &Room{
			id:           roomId,
			singleplayer: contains(spRooms, roomId),
			conditions:   getRoomConditions(roomId),
			minigames:    getRoomMinigames(roomId),
		}
	}
}

func handleRoom(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-Websocket-Protocol": {r.Header.Get("Sec-Websocket-Protocol")}})
	if err != nil {
		log.Println(err)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		return
	}

	idInt, err := strconv.Atoi(id)
	if err != nil {
		log.Println(err)
		return
	}

	var playerToken string
	if token := r.URL.Query().Get("token"); len(token) == 32 {
		playerToken = token
	}

	joinRoomWs(conn, getIp(r), playerToken, idInt)
}

func joinRoomWs(conn *websocket.Conn, ip string, token string, roomId int) {
	// we don't need the value of room until later but it would be silly to do
	// the database lookups then close the socket after due to a bad room id
	room, ok := rooms[roomId]
	if !ok {
		return
	}

	var uuid string
	if token != "" {
		uuid = getUuidFromToken(token)
	}

	if uuid == "" {
		uuid, _, _ = getOrCreatePlayerData(ip)
	}

	client := &RoomClient{
		conn:      conn,
		writerEnd: make(chan bool, 1),
		send:      make(chan []byte, 256),
		receive:   make(chan []byte, 8),
		key:       serverSecurity.NewClientKey(),
	}

	// use 0000 as a placeholder since client.mapId isn't set until later
	if s, ok := clients.Load(uuid); ok {
		session := s.(*SessionClient)
		if session.rClient != nil {
			session.rClient.disconnect()
		}

		session.rClient = client
		client.sClient = session
	} else {
		writeErrLog(uuid, "0000", "player has no session")
		return
	}

	if tags, err := getPlayerTags(uuid); err != nil {
		writeErrLog(uuid, "0000", "failed to read player tags")
	} else {
		client.tags = tags
	}

	// start msgWriter first otherwise the call to syncRoomState in joinRoom
	// will make the send channel full and start blocking the goroutine
	go client.msgWriter()

	// send client info about itself
	client.send <- buildMsg("s", client.sClient.id, int(client.key), uuid, client.sClient.rank, client.sClient.account, client.sClient.badge, client.sClient.medals[:])

	// register client to room
	client.joinRoom(room)

	// start msgProcessor and msgReader after so a client can't send packets
	// before they're in a room and try to crash the server
	go client.msgProcessor()
	go client.msgReader()

	// send synced picture names, picture prefixes, and battle animation ids
	if len(gameAssets.PictureNames) != 0 {
		client.send <- buildMsg("pns", 0, gameAssets.PictureNames)
	}
	if len(gameAssets.PicturePrefixes) != 0 {
		client.send <- buildMsg("pns", 1, gameAssets.PicturePrefixes)
	}
	if len(gameAssets.BattleAnimIds) != 0 {
		client.send <- buildMsg("bas", gameAssets.BattleAnimIds)
	}

	writeLog(client.sClient.uuid, client.mapId, "connect", 200)
}

func (c *RoomClient) joinRoom(room *Room) {
	c.room = room

	c.reset()

	c.send <- buildMsg("ri", c.room.id) // tell client they've switched rooms serverside

	if !c.room.singleplayer {
		c.getRoomPlayerData()

		room.clients = append(room.clients, c)

		// tell everyone that a new client has connected
		c.broadcast(buildMsg("c", c.sClient.id, c.sClient.uuid, c.sClient.rank, c.sClient.account, c.sClient.badge, c.sClient.medals[:])) // user %id% has connected message

		// send name of client
		if c.sClient.name != "" {
			c.broadcast(buildMsg("name", c.sClient.id, c.sClient.name))
		}
	}

	if c.sClient.account {
		c.getRoomEventData()
	}
}

func (c *RoomClient) leaveRoom() {
	// setting c.room to nil could cause a nil pointer dereference
	// so we let joinRoom update it

	for clientIdx, client := range c.room.clients {
		if client != c {
			continue
		}

		c.room.clients[clientIdx] = c.room.clients[len(c.room.clients)-1]
		c.room.clients = c.room.clients[:len(c.room.clients)-1]
	}

	c.broadcast(buildMsg("d", c.sClient.id)) // user %id% has disconnected message
}

func (c *RoomClient) broadcast(msg []byte) {
	for _, client := range c.room.clients {
		if client == c && !(len(msg) > 3 && string(msg[:3]) == "say") {
			continue
		}

		select {
		case client.send <- msg:
		default:
			writeErrLog(c.sClient.uuid, c.mapId, "send channel is full")
		}
	}
}

func (c *RoomClient) processMsgs(msg []byte) (errs []error) {
	if len(msg) < 8 {
		return append(errs, errors.New("bad request size"))
	}

	if !serverSecurity.VerifySignature(c.key, msg) {
		return append(errs, errors.New("bad signature"))
	}

	if !serverSecurity.VerifyCounter(&c.counter, msg) {
		return append(errs, errors.New("bad counter"))
	}

	msg = msg[8:]

	if !utf8.Valid(msg) {
		return append(errs, errors.New("invalid utf8"))
	}

	// message processing
	for _, msgStr := range strings.Split(string(msg), mdelim) {
		if err := c.processMsg(msgStr); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (c *RoomClient) processMsg(msgStr string) (err error) {
	switch msgFields := strings.Split(msgStr, delim); msgFields[0] {
	case "sr": // switch room
		err = c.handleSr(msgFields)
	case "m", "tp": // moved / teleported to x y
		err = c.handleM(msgFields)
	case "jmp": // jumped to x y
		err = c.handleJmp(msgFields)
	case "f": // change facing direction
		err = c.handleF(msgFields)
	case "spd": // change my speed to spd
		err = c.handleSpd(msgFields)
	case "spr": // change my sprite
		err = c.handleSpr(msgFields)
	case "fl", "rfl": // player flash / repeating player flash
		err = c.handleFl(msgFields)
	case "rrfl": // remove repeating player flash
		err = c.handleRrfl()
	case "h": // change sprite visibility
		err = c.handleH(msgFields)
	case "sys": // change my system graphic
		err = c.handleSys(msgFields)
	case "se": // play sound effect
		err = c.handleSe(msgFields)
	case "ap", "mp": // add picture / move picture
		err = c.handleP(msgFields)
	case "rp": // remove picture
		err = c.handleRp(msgFields)
	case "ba": // battle animation
		err = c.handleBa(msgFields)
	case "say":
		err = c.handleSay(msgFields)
	case "ss": // sync switch
		err = c.handleSs(msgFields)
	case "sv": // sync variable
		err = c.handleSv(msgFields)
	case "sev":
		err = c.handleSev(msgFields)
	default:
		err = errors.New("unknown message type")
	}
	if err != nil {
		return err
	}

	writeLog(c.sClient.uuid, c.mapId, msgStr, 200)

	return nil
}

func (c *RoomClient) getRoomPlayerData() {
	// send the new client info about the game state
	for _, otherClient := range c.room.clients {
		if otherClient == c {
			continue
		}

		c.send <- buildMsg("c", otherClient.sClient.id, otherClient.sClient.uuid, otherClient.sClient.rank, otherClient.sClient.account, otherClient.sClient.badge, otherClient.sClient.medals[:])
		c.send <- buildMsg("m", otherClient.sClient.id, otherClient.x, otherClient.y)
		if otherClient.facing > 0 {
			c.send <- buildMsg("f", otherClient.sClient.id, otherClient.facing)
		}
		c.send <- buildMsg("spd", otherClient.sClient.id, otherClient.spd)
		if otherClient.sClient.name != "" {
			c.send <- buildMsg("name", otherClient.sClient.id, otherClient.sClient.name)
		}
		if otherClient.sClient.spriteIndex >= 0 {
			c.send <- buildMsg("spr", otherClient.sClient.id, otherClient.sClient.spriteName, otherClient.sClient.spriteIndex) // if the other client sent us valid sprite and index before
		}
		if otherClient.repeatingFlash {
			c.send <- buildMsg("rfl", otherClient.sClient.id, otherClient.flash[:])
		}
		if otherClient.hidden {
			c.send <- buildMsg("h", otherClient.sClient.id, "1")
		}
		if otherClient.sClient.systemName != "" {
			c.send <- buildMsg("sys", otherClient.sClient.id, otherClient.sClient.systemName)
		}
		for picId, pic := range otherClient.pictures {
			c.send <- buildMsg("ap", otherClient.sClient.id, picId, pic.positionX, pic.positionY, pic.mapX, pic.mapY, pic.panX, pic.panY, pic.magnify, pic.topTrans, pic.bottomTrans, pic.red, pic.blue, pic.green, pic.saturation, pic.effectMode, pic.effectPower, pic.name, pic.useTransparentColor, pic.fixedToMap)
		}
	}
}

func (c *RoomClient) getRoomEventData() {
	c.checkRoomConditions("", "")

	for _, minigame := range c.room.minigames {
		if minigame.Dev && c.sClient.rank < 1 {
			continue
		}
		score, err := getPlayerMinigameScore(c.sClient.uuid, minigame.Id)
		if err != nil {
			writeErrLog(c.sClient.uuid, c.mapId, "failed to read player minigame score for "+minigame.Id)
		}
		c.minigameScores = append(c.minigameScores, score)
		varSyncType := 1
		if minigame.InitialVarSync {
			varSyncType = 2
		}
		c.send <- buildMsg("sv", minigame.VarId, varSyncType)
	}

	// send variable sync request for vending machine expeditions
	if c.room.id != currentEventVmMapId {
		return
	}

	if eventIds, hasVms := eventVms[c.room.id]; hasVms {
		for _, eventId := range eventIds {
			if eventId != currentEventVmEventId {
				continue
			}
			c.send <- buildMsg("sev", eventId, "1")
		}
	}
}
