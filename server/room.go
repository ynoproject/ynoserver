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
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/fasthttp/websocket"
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
	logInitTask("rooms")

	for _, roomId := range roomIds {
		rooms[roomId] = &Room{
			id:           roomId,
			singleplayer: slices.Contains(spRooms, roomId),
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
		conn:   conn,
		outbox: make(chan []byte, 256),
		key:    serverSecurity.NewClientKey(),
	}

	if session, ok := clients.Load(uuid); ok {
		if session.roomC != nil {
			session.roomC.cancel()
		}

		session.roomC = client
		client.session = session
	} else {
		// use 0000 as a placeholder since client.mapId isn't set until later
		writeErrLog(uuid, "0000", "player has no session")
		return
	}

	client.ctx, client.cancel = context.WithCancel(client.session.ctx)

	if tags, _, err := getPlayerTags(uuid); err != nil {
		writeErrLog(uuid, "0000", "failed to read player tags")
	} else {
		client.tags = tags
	}

	go client.msgWriter()

	// send client info about itself
	client.outbox <- buildMsg("s", client.session.id, int(client.key), uuid, client.session.rank, client.session.account, client.session.badge, client.session.medals[:])

	// register client to room
	client.joinRoom(room)

	go client.msgReader()

	// send synced picture names, picture prefixes, and battle animation ids
	if len(config.pictures) != 0 {
		client.outbox <- buildMsg("pns", 0, config.pictures)
	}
	if len(config.picturePrefixes) != 0 {
		client.outbox <- buildMsg("pns", 1, config.picturePrefixes)
	}
	if len(config.battleAnimIds) != 0 {
		client.outbox <- buildMsg("bas", config.battleAnimIds)
	}

	if config.gameName == "unconscious" {
		didJoinRoomUnconscious(client)
	}

	writeLog(client.session.uuid, client.mapId, "connect", 200)
}

func (c *RoomClient) joinRoom(room *Room) {
	c.room = room

	c.reset()

	c.outbox <- buildMsg("ri", c.room.id) // tell client they've switched rooms serverside

	if config.gameName == "2kki" && c.session.rank == 0 {
		c.outbox <- buildMsg("ss", 11, 2)
	}

	if !c.room.singleplayer {
		c.getRoomPlayerData()

		room.clients = append(room.clients, c)

		// tell everyone that a new client has connected
		c.broadcast(buildMsg("c", c.session.id, c.session.uuid, c.session.rank, c.session.account, c.session.badge, c.session.medals[:])) // user %id% has connected message

		// send name of client
		if c.session.name != "" {
			c.broadcast(buildMsg("name", c.session.id, c.session.name))
		}
	}

	if c.session.account {
		c.getRoomEventData()
	}
}

func (c *RoomClient) leaveRoom() {
	// setting c.room to nil could cause a nil pointer dereference
	// so we let joinRoom update it

	for i, client := range c.room.clients {
		if client != c {
			continue
		}

		c.room.clients[i] = c.room.clients[len(c.room.clients)-1]
		c.room.clients = c.room.clients[:len(c.room.clients)-1]
	}

	c.broadcast(buildMsg("d", c.session.id)) // user %id% has disconnected message
}

func (c *RoomClient) broadcast(msg []byte) {
	for _, client := range c.room.clients {
		if client == c {
			continue
		}

		if (client.session.private || c.session.private) && ((c.session.partyId == 0 || client.session.partyId != c.session.partyId) && !client.session.onlineFriends[c.session.uuid]) {
			continue
		}

		select {
		case client.outbox <- msg:
		default:
			writeErrLog(c.session.uuid, c.mapId, "send channel is full")
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
	var updateGameActivity bool

	switch msgFields := strings.Split(msgStr, delim); msgFields[0] {
	case "sr": // switch room
		err = c.handleSr(msgFields)
		updateGameActivity = true
	case "m", "tp", "jmp": // moved / teleported / jumped to x y
		err = c.handleM(msgFields)
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
	case "tr": // change transparency
		err = c.handleTr(msgFields)
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

	if updateGameActivity {
		err = c.session.updatePlayerGameActivity(true)
		if err != nil {
			writeErrLog(c.session.uuid, c.mapId, err.Error())
		}
	}

	writeLog(c.session.uuid, c.mapId, msgStr, 200)

	return nil
}

func (c *RoomClient) getRoomPlayerData() {
	// send the new client info about the game state
	for _, client := range c.room.clients {
		c.getPlayerData(client)
	}
}

func (c *RoomClient) getPlayerData(client *RoomClient) {
	if client == c {
		return
	}

	if (client.session.private || c.session.private) && ((c.session.partyId == 0 || client.session.partyId != c.session.partyId) && !client.session.onlineFriends[c.session.uuid]) {
		return
	}

	c.outbox <- buildMsg("c", client.session.id, client.session.uuid, client.session.rank, client.session.account, client.session.badge, client.session.medals[:])

	// client.x and client.y get set at the same time
	// only one needs to be checked
	if client.x != -1 {
		c.outbox <- buildMsg("m", client.session.id, client.x, client.y)
	}
	if client.facing != 0 {
		c.outbox <- buildMsg("f", client.session.id, client.facing)
	}
	if client.speed != 0 {
		c.outbox <- buildMsg("spd", client.session.id, client.speed)
	}
	if client.session.name != "" {
		c.outbox <- buildMsg("name", client.session.id, client.session.name)
	}
	if client.session.spriteIndex != -1 {
		c.outbox <- buildMsg("spr", client.session.id, client.session.sprite, client.session.spriteIndex) // if the other client sent us valid sprite and index before
	}
	if client.repeatingFlash {
		c.outbox <- buildMsg("rfl", client.session.id, client.flash[:])
	}
	if client.transparency != 0 {
		c.outbox <- buildMsg("tr", client.session.id, client.transparency)
	}
	if client.hidden {
		c.outbox <- buildMsg("h", client.session.id, 1)
	}
	if client.session.system != "" {
		c.outbox <- buildMsg("sys", client.session.id, client.session.system)
	}
	for i, pic := range client.pictures {
		if pic != nil {
			c.outbox <- buildMsg("ap", client.session.id, i+1, pic.posX, pic.posY, pic.mapX, pic.mapY, pic.panX, pic.panY, pic.magnify, pic.topTrans, pic.bottomTrans, pic.red, pic.blue, pic.green, pic.saturation, pic.effectMode, pic.effectPower, pic.name, pic.useTransparentColor, pic.fixedToMap, pic.spritesheetCols, pic.spritesheetRows, pic.spritesheetFrame, pic.spritesheetSpeed, pic.spritesheetPlayOnce, pic.mapLayer, pic.battleLayer, pic.flags, pic.blendMode, pic.flipX, pic.flipY, pic.origin)
		}
	}
}

func (c *RoomClient) getRoomEventData() {
	c.checkRoomConditions("", "")

	for _, minigame := range c.room.minigames {
		if minigame.Dev && c.session.rank < 1 {
			continue
		}
		score, err := getPlayerMinigameScore(c.session.uuid, minigame.Id)
		if err != nil {
			writeErrLog(c.session.uuid, c.mapId, "failed to read player minigame score for "+minigame.Id)
		}
		c.minigameScores = append(c.minigameScores, score)
		varSyncType := 1
		if minigame.InitialVarSync {
			varSyncType = 2
		}
		c.outbox <- buildMsg("sv", minigame.VarId, varSyncType)
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
			c.outbox <- buildMsg("sev", eventId, 1)
		}
	}
}
