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
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

const (
	delim  = "\uffff"
	mdelim = "\ufffe"
)

var (
	delimBytes = []byte("\uffff")

	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	rooms = make(map[int]*Room)
)

// Room maintains the set of active clients and broadcasts messages to the
// clients.
type Room struct {
	// Registered clients.
	clients sync.Map

	id           int
	singleplayer bool

	conditions []*Condition

	minigameConfigs []*MinigameConfig
}

func createRooms(roomIds []int, spRooms []int) {
	for _, roomId := range roomIds {
		rooms[roomId] = &Room{
			id:              roomId,
			singleplayer:    contains(spRooms, roomId),
			conditions:      getRoomConditions(roomId),
			minigameConfigs: getRoomMinigameConfigs(roomId),
		}
	}
}

func handleRoom(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-Websocket-Protocol": {r.Header.Get("Sec-Websocket-Protocol")}})
	if err != nil {
		log.Println(err)
		return
	}

	id, ok := r.URL.Query()["id"]
	if !ok {
		return
	}

	idInt, err := strconv.Atoi(id[0])
	if err != nil {
		log.Println(err)
		return
	}

	var playerToken string
	if token, ok := r.URL.Query()["token"]; ok && len(token[0]) == 32 {
		playerToken = token[0]
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
		send:      make(chan []byte, 16),
		receive:   make(chan []byte, 16),
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
	client.sendMsg("s", client.sClient.id, uuid, client.sClient.rank, client.sClient.account, client.sClient.badge)

	// register client to room
	client.joinRoom(room)

	// start msgProcessor and msgReader after so a client can't send packets
	// before they're in a room and try to crash the server
	go client.msgProcessor()
	go client.msgReader()

	// TODO: create these arrays once and store them
	// convert PictureNames to a string array so we can send it
	var picNames []string
	for picName := range gameAssets.PictureNames {
		picNames = append(picNames, picName)
	}

	// send synced picture names and picture prefixes
	if len(picNames) > 0 {
		client.sendMsg("pns", 0, picNames)
	}
	if len(gameAssets.PicturePrefixes) > 0 {
		client.sendMsg("pns", 1, gameAssets.PicturePrefixes)
	}

	// convert BattleAnimIds to an int array so we can send it
	var battleAnimIds []int
	for battleAnimId := range gameAssets.BattleAnimIds {
		battleAnimIds = append(battleAnimIds, battleAnimId)
	}

	// send synced battle animation ids
	if len(battleAnimIds) > 0 {
		client.sendMsg("bas", battleAnimIds)
	}

	writeLog(client.sClient.uuid, client.mapId, "connect", 200)
}

func (c *RoomClient) joinRoom(room *Room) {
	c.room = room

	c.reset()

	c.sendMsg("ri", c.room.id, int(c.key))

	room.clients.Store(c, nil)

	c.syncRoomState()
}

func (c *RoomClient) leaveRoom() {
	// setting c.room to nil could cause a nil pointer dereference
	// so we let joinRoom update it

	c.room.clients.Delete(c)

	c.broadcast("d", c.sClient.id) // user %id% has disconnected message
}

func (sender *RoomClient) broadcast(segments ...any) {
	if sender.room.singleplayer {
		return
	}

	sender.room.clients.Range(func(k, _ any) bool {
		client := k.(*RoomClient)
		if client == sender && segments[0].(string) != "say" {
			return true
		}

		client.sendMsg(segments...)

		return true
	})
}

func (sender *RoomClient) processMsgs(msg []byte) (errs []error) {
	if len(msg) < 8 {
		return append(errs, errBadReqSize)
	}

	if !serverSecurity.VerifySignature(sender.key, msg) {
		return append(errs, errBadSignature)
	}

	if !serverSecurity.VerifyCounter(&sender.counter, msg) {
		return append(errs, errBadCounter)
	}

	msg = msg[8:]

	if !utf8.Valid(msg) {
		return append(errs, errInvalidUTF8)
	}

	// message processing
	for _, msgStr := range strings.Split(string(msg), mdelim) {
		if err := sender.processMsg(msgStr); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (sender *RoomClient) processMsg(msgStr string) (err error) {
	switch msgFields := strings.Split(msgStr, delim); msgFields[0] {
	case "sr": // switch room
		err = sender.handleSr(msgFields)
	case "m", "tp": // moved / teleported to x y
		err = sender.handleM(msgFields)
	case "f": // change facing direction
		err = sender.handleF(msgFields)
	case "spd": // change my speed to spd
		err = sender.handleSpd(msgFields)
	case "spr": // change my sprite
		err = sender.handleSpr(msgFields)
	case "fl", "rfl": // player flash / repeating player flash
		err = sender.handleFl(msgFields)
	case "rrfl": // remove repeating player flash
		err = sender.handleRrfl()
	case "h": // change sprite visibility
		err = sender.handleH(msgFields)
	case "sys": // change my system graphic
		err = sender.handleSys(msgFields)
	case "se": // play sound effect
		err = sender.handleSe(msgFields)
	case "ap", "mp": // add picture / move picture
		err = sender.handleP(msgFields)
	case "rp": // remove picture
		err = sender.handleRp(msgFields)
	case "ba": // battle animation
		err = sender.handleBa(msgFields)
	case "say":
		err = sender.handleSay(msgFields)
	case "ss": // sync switch
		err = sender.handleSs(msgFields)
	case "sv": // sync variable
		err = sender.handleSv(msgFields)
	case "sev":
		err = sender.handleSev(msgFields)
	default:
		err = errUnkMsgType
	}
	if err != nil {
		return err
	}

	writeLog(sender.sClient.uuid, sender.mapId, msgStr, 200)

	return nil
}

func (client *RoomClient) syncRoomState() {
	if !client.room.singleplayer {
		// tell everyone that a new client has connected
		client.broadcast("c", client.sClient.id, client.sClient.uuid, client.sClient.rank, client.sClient.account, client.sClient.badge) // user %id% has connected message

		// send name of client
		if client.sClient.name != "" {
			client.broadcast("name", client.sClient.id, client.sClient.name)
		}

		// send the new client info about the game state
		client.room.clients.Range(func(k, _ any) bool {
			otherClient := k.(*RoomClient)
			if otherClient == client {
				return true
			}

			client.sendMsg("c", otherClient.sClient.id, otherClient.sClient.uuid, otherClient.sClient.rank, otherClient.sClient.account, otherClient.sClient.badge)
			client.sendMsg("m", otherClient.sClient.id, otherClient.x, otherClient.y)
			if otherClient.facing > 0 {
				client.sendMsg("f", otherClient.sClient.id, otherClient.facing)
			}
			client.sendMsg("spd", otherClient.sClient.id, otherClient.spd)
			if otherClient.sClient.name != "" {
				client.sendMsg("name", otherClient.sClient.id, otherClient.sClient.name)
			}
			if otherClient.sClient.spriteIndex >= 0 {
				client.sendMsg("spr", otherClient.sClient.id, otherClient.sClient.spriteName, otherClient.sClient.spriteIndex) // if the other client sent us valid sprite and index before
			}
			if otherClient.repeatingFlash {
				client.sendMsg("rfl", otherClient.sClient.id, otherClient.flash[:])
			}
			if otherClient.hidden {
				client.sendMsg("h", otherClient.sClient.id, "1")
			}
			if otherClient.sClient.systemName != "" {
				client.sendMsg("sys", otherClient.sClient.id, otherClient.sClient.systemName)
			}
			for picId, pic := range otherClient.pictures {
				client.sendMsg("ap", otherClient.sClient.id, picId, pic.positionX, pic.positionY, pic.mapX, pic.mapY, pic.panX, pic.panY, pic.magnify, pic.topTrans, pic.bottomTrans, pic.red, pic.blue, pic.green, pic.saturation, pic.effectMode, pic.effectPower, pic.name, pic.useTransparentColor, pic.fixedToMap)
			}

			return true
		})
	}

	// if you need an account to do the stuff after this, why bother?
	if !client.sClient.account {
		return
	}

	client.checkRoomConditions("", "")

	for _, minigame := range client.room.minigameConfigs {
		score, err := getPlayerMinigameScore(client.sClient.uuid, minigame.MinigameId)
		if err != nil {
			writeErrLog(client.sClient.uuid, client.mapId, "failed to read player minigame score for "+minigame.MinigameId)
		}
		client.minigameScores = append(client.minigameScores, score)
		varSyncType := 1
		if minigame.InitialVarSync {
			varSyncType = 2
		}
		client.sendMsg("sv", minigame.VarId, varSyncType)
	}

	// send variable sync request for vending machine expeditions
	if client.room.id != currentEventVmMapId {
		return
	}

	if eventIds, hasVms := eventVms[client.room.id]; hasVms {
		for _, eventId := range eventIds {
			if eventId != currentEventVmEventId {
				continue
			}
			client.sendMsg("sev", eventId, "1")
		}
	}
}
