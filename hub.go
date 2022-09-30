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
	"fmt"
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
	hubClients sync.Map
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	hubs []*Hub
)

type ConnInfo struct {
	Connect *websocket.Conn
	Ip      string
	Token   string
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients sync.Map

	// Inbound messages from the clients.
	processMsgCh chan *Message

	// Connection requests from the clients.
	connect chan *ConnInfo

	// Unregister requests from clients.
	unregister chan *Client

	roomId       int
	singleplayer bool

	conditions []*Condition

	minigameConfigs []*MinigameConfig
}

func createAllHubs(roomIds []int, spRooms []int) {
	for _, roomId := range roomIds {
		addHub(roomId, contains(spRooms, roomId))
	}
}

func addHub(roomId int, singleplayer bool) {
	hub := newHub(roomId, singleplayer)
	hubs = append(hubs, hub)
	go hub.run()
}

func newHub(roomId int, singleplayer bool) *Hub {
	return &Hub{
		processMsgCh:    make(chan *Message, 16),
		connect:         make(chan *ConnInfo, 4),
		unregister:      make(chan *Client, 4),
		roomId:          roomId,
		singleplayer:    singleplayer,
		conditions:      getHubConditions(roomId),
		minigameConfigs: getHubMinigameConfigs(roomId),
	}
}

func (h *Hub) run() {
	http.HandleFunc("/"+strconv.Itoa(h.roomId), h.serve)
	for {
		select {
		case conn := <-h.connect:
			client := &Client{
				hub:         h,
				conn:        conn.Connect,
				terminate:   make(chan bool, 1),
				send:        make(chan []byte, 16),
				key:         generateKey(),
				pictures:    make(map[int]*Picture),
				mapId:       fmt.Sprintf("%04d", h.roomId),
				switchCache: make(map[int]bool),
				varCache:    make(map[int]int),
			}

			var uuid string
			if conn.Token != "" {
				uuid = getUuidFromToken(conn.Token)
			}

			if uuid == "" {
				uuid, _, _ = getOrCreatePlayerData(conn.Ip)
			}

			if s, ok := sessionClients.Load(uuid); ok {
				session := s.(*SessionClient)

				if session.bound {
					writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "session in use")
					continue
				}

				session.bound = true

				client.session = session
			} else {
				writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "player has no session")
				continue
			}

			for {
				client.id++

				if _, ok := h.clients.Load(client.id); !ok {
					break
				}
			}

			if tags, err := getPlayerTags(uuid); err != nil {
				writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "failed to read player tags")
			} else {
				client.tags = tags
			}

			go client.writePump()
			go client.readPump()

			client.send <- []byte("s" + delim + strconv.Itoa(client.id) + delim + strconv.FormatUint(uint64(client.key), 10) + delim + uuid + delim + strconv.Itoa(client.session.rank) + delim + btoa(client.session.account) + delim + client.session.badge) //"your id is %id%" message

			//register client in the structures
			h.clients.Store(client.id, client)
			hubClients.Store(uuid, client)

			writeLog(conn.Ip, strconv.Itoa(h.roomId), "connect", 200)
		case client := <-h.unregister:
			close(client.terminate)

			client.session.bound = false

			h.clients.Delete(client.id)
			hubClients.Delete(client.session.uuid)

			h.broadcast([]byte("d" + delim + strconv.Itoa(client.id))) //user %id% has disconnected message

			writeLog(client.session.ip, strconv.Itoa(h.roomId), "disconnect", 200)

			client.valid = false // TODO: Find a better way of doing this. Prevent a Hub deadlock while trying to send to a full send channel.
		case message := <-h.processMsgCh:
			if errs := h.processMsgs(message); len(errs) > 0 {
				for _, err := range errs {
					writeErrLog(message.sender.session.ip, strconv.Itoa(h.roomId), err.Error())
				}
			}
		}
	}
}

// serve handles websocket requests from the peer.
func (h *Hub) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-Websocket-Protocol": {r.Header.Get("Sec-Websocket-Protocol")}})
	if err != nil {
		log.Println(err)
		return
	}

	var playerToken string
	if token, ok := r.URL.Query()["token"]; ok && len(token[0]) == 32 {
		playerToken = token[0]
	}

	h.connect <- &ConnInfo{Connect: conn, Ip: getIp(r), Token: playerToken}
}

func (h *Hub) broadcast(data []byte) {
	if h.singleplayer {
		return
	}

	h.clients.Range(func(_, v any) bool {
		client := v.(*Client)

		if !client.valid {
			return true
		}

		client.send <- data

		return true
	})
}

func (h *Hub) processMsgs(msg *Message) []error {
	var errs []error

	if len(msg.data) < 8 || len(msg.data) > 4096 {
		return append(errs, errors.New("bad request size"))
	}

	if !verifySignature(msg.sender.key, msg.data) {
		return append(errs, errors.New("bad signature"))
	}

	if !verifyCounter(&msg.sender.counter, msg.data) {
		return append(errs, errors.New("bad counter"))
	}

	msg.data = msg.data[8:]

	for _, v := range msg.data {
		if v < 32 {
			return append(errs, errors.New("bad byte sequence"))
		}
	}

	if !utf8.Valid(msg.data) {
		return append(errs, errors.New("invalid UTF-8"))
	}

	//message processing
	for _, msgStr := range strings.Split(string(msg.data), mdelim) {
		terminate, err := h.processMsg(msgStr, msg.sender)
		if err != nil {
			errs = append(errs, err)
		}
		if terminate {
			break
		}
	}

	return errs
}

func (h *Hub) processMsg(msgStr string, sender *Client) (bool, error) {
	err := errors.New(msgStr)
	msgFields := strings.Split(msgStr, delim)

	if len(msgFields) == 0 {
		return false, err
	}

	var terminate bool

	if !sender.valid {
		if msgFields[0] == "ident" {
			err = h.handleIdent(msgFields, sender)
		}
	} else {
		switch msgFields[0] {
		case "m": //moved to x y
			fallthrough
		case "tp": //teleported to x y
			err = h.handleM(msgFields, sender)
		case "f": //change facing direction
			err = h.handleF(msgFields, sender)
		case "spd": //change my speed to spd
			err = h.handleSpd(msgFields, sender)
		case "spr": //change my sprite
			err = h.handleSpr(msgFields, sender)
		case "fl": //player flash
			fallthrough
		case "rfl": //repeating player flash
			err = h.handleFl(msgFields, sender)
		case "rrfl": //remove repeating player flash
			err = h.handleRrfl(sender)
		case "h": //change sprite visibility
			err = h.handleH(msgFields, sender)
		case "sys": //change my system graphic
			err = h.handleSys(msgFields, sender)
		case "se": //play sound effect
			err = h.handleSe(msgFields, sender)
		case "ap": // picture shown
			fallthrough
		case "mp": // picture moved
			err = h.handleP(msgFields, sender)
		case "rp": // picture erased
			err = h.handleRp(msgFields, sender)
		case "say":
			err = h.handleSay(msgFields, sender)
			terminate = true
		case "ss": // sync switch
			err = h.handleSs(msgFields, sender)
		case "sv": // sync variable
			err = h.handleSv(msgFields, sender)
		case "sev":
			err = h.handleSev(msgFields, sender)
		default:
			err = errors.New("unknown message type")
		}
	}

	if err != nil {
		return false, err
	}

	writeLog(sender.session.ip, strconv.Itoa(h.roomId), msgStr, 200)

	return terminate, nil
}

func (h *Hub) handleValidClient(client *Client) {
	if !h.singleplayer {
		//tell everyone that a new client has connected
		h.broadcast([]byte("c" + delim + strconv.Itoa(client.id) + delim + client.session.uuid + delim + strconv.Itoa(client.session.rank) + delim + btoa(client.session.account) + delim + client.session.badge)) //user %id% has connected message

		//send name of client
		if client.session.name != "" {
			h.broadcast([]byte("name" + delim + strconv.Itoa(client.id) + delim + client.session.name))
		}

		//send the new client info about the game state
		h.clients.Range(func(_, v any) bool {
			otherClient := v.(*Client)

			if !otherClient.valid {
				return true
			}

			client.send <- []byte("c" + delim + strconv.Itoa(otherClient.id) + delim + otherClient.session.uuid + delim + strconv.Itoa(otherClient.session.rank) + delim + btoa(otherClient.session.account) + delim + otherClient.session.badge)
			client.send <- []byte("m" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(otherClient.x) + delim + strconv.Itoa(otherClient.y))
			if otherClient.facing > 0 {
				client.send <- []byte("f" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(otherClient.facing))
			}
			client.send <- []byte("spd" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(otherClient.spd))
			if otherClient.session.name != "" {
				client.send <- []byte("name" + delim + strconv.Itoa(otherClient.id) + delim + otherClient.session.name)
			}
			if otherClient.session.spriteIndex >= 0 { //if the other client sent us valid sprite and index before
				client.send <- []byte("spr" + delim + strconv.Itoa(otherClient.id) + delim + otherClient.session.spriteName + delim + strconv.Itoa(otherClient.session.spriteIndex))
			}
			if otherClient.repeatingFlash {
				client.send <- []byte("rfl" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(otherClient.flash[0]) + delim + strconv.Itoa(otherClient.flash[1]) + delim + strconv.Itoa(otherClient.flash[2]) + delim + strconv.Itoa(otherClient.flash[3]) + delim + strconv.Itoa(otherClient.flash[4]))
			}
			if otherClient.hidden {
				client.send <- []byte("h" + delim + strconv.Itoa(otherClient.id) + delim + "1")
			}
			if otherClient.session.systemName != "" {
				client.send <- []byte("sys" + delim + strconv.Itoa(otherClient.id) + delim + otherClient.session.systemName)
			}
			for picId, pic := range otherClient.pictures {
				client.send <- []byte("ap" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(picId) + delim + strconv.Itoa(pic.positionX) + delim + strconv.Itoa(pic.positionY) + delim + strconv.Itoa(pic.mapX) + delim + strconv.Itoa(pic.mapY) + delim + strconv.Itoa(pic.panX) + delim + strconv.Itoa(pic.panY) + delim + strconv.Itoa(pic.magnify) + delim + strconv.Itoa(pic.topTrans) + delim + strconv.Itoa(pic.bottomTrans) + delim + strconv.Itoa(pic.red) + delim + strconv.Itoa(pic.blue) + delim + strconv.Itoa(pic.green) + delim + strconv.Itoa(pic.saturation) + delim + strconv.Itoa(pic.effectMode) + delim + strconv.Itoa(pic.effectPower) + delim + pic.name + delim + btoa(pic.useTransparentColor) + delim + btoa(pic.fixedToMap))
			}

			return true
		})
	}

	checkHubConditions(h, client, "", "")

	for _, minigame := range h.minigameConfigs {
		score, err := getPlayerMinigameScore(client.session.uuid, minigame.MinigameId)
		if err != nil {
			writeErrLog(client.session.ip, strconv.Itoa(h.roomId), "failed to read player minigame score for "+minigame.MinigameId)
		}
		client.minigameScores = append(client.minigameScores, score)
		varSyncType := 1
		if minigame.InitialVarSync {
			varSyncType = 2
		}
		client.send <- []byte("sv" + delim + strconv.Itoa(minigame.VarId) + delim + strconv.Itoa(varSyncType))
	}

	// send variable sync request for vending machine expeditions
	if h.roomId != currentEventVmMapId {
		return
	}

	if eventIds, hasVms := eventVms[h.roomId]; hasVms {
		for _, eventId := range eventIds {
			if eventId != currentEventVmEventId {
				continue
			}
			client.send <- []byte("sev" + delim + strconv.Itoa(eventId) + delim + "1")
		}
	}
}
