package main

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/thanhpk/randstr"
)

const (
	maxID         = 512
	paramDelimStr = "\uffff"
	msgDelimStr   = "\ufffe"
)

var (
	allClients = make(map[string]*Client)
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	isOkString = regexp.MustCompile("^[A-Za-z0-9]+$").MatchString

	hubs []*Hub

	config     Config
	conditions map[string]map[string]*Condition
	badges     map[string]map[string]*Badge
	db         *sql.DB
)

type ConnInfo struct {
	Connect *websocket.Conn
	Ip      string
	Session string
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// True if the id is in use
	id map[int]bool

	// Inbound messages from the clients.
	processMsgCh chan *Message

	// Connection requests from the clients.
	connect chan *ConnInfo

	// Unregister requests from clients.
	unregister chan *Client

	roomName string
	singleplayer bool

	conditions []*Condition

	minigameConfigs []*MinigameConfig
}

func createAllHubs(roomNames []string, badRooms []string) {
	for _, roomName := range roomNames {
		addHub(roomName, contains(badRooms, roomName))
	}
}

func addHub(roomName string, singleplayer bool) {
	hub := newHub(roomName, singleplayer)
	hubs = append(hubs, hub)
	go hub.run()
}

func newHub(roomName string, singleplayer bool) *Hub {
	return &Hub{
		processMsgCh:    make(chan *Message),
		connect:         make(chan *ConnInfo),
		unregister:      make(chan *Client),
		clients:         make(map[*Client]bool),
		id:              make(map[int]bool),
		roomName:        roomName,
		singleplayer:    singleplayer,
		conditions:      getHubConditions(roomName),
		minigameConfigs: getHubMinigameConfigs(roomName),
	}
}

func (h *Hub) run() {
	http.HandleFunc("/"+h.roomName, h.serveWs)
	for {
		select {
		case conn := <-h.connect:
			var uuid string
			var name string
			var rank int
			badge := "null"

			var isBanned bool
			var isLoggedIn bool

			if conn.Session != "" {
				uuid, name, rank, badge, isBanned = readPlayerDataFromSession(conn.Session)
				if uuid != "" { //if we got a uuid back then we're logged in
					isLoggedIn = true
				}
				if badge == "" {
					badge = "null"
				}
			}

			if !isLoggedIn {
				uuid, rank, isBanned = readPlayerData(conn.Ip)
			}

			if isBanned && rank < 1 {
				writeErrLog(conn.Ip, h.roomName, "player is banned")
				continue
			}

			var same_ip int
			ip_limit := 3
			for otherClient := range h.clients {
				if otherClient.ip == conn.Ip {
					same_ip++
				}
			}
			if same_ip >= ip_limit {
				writeErrLog(conn.Ip, h.roomName, "too many connections")
				continue //don't bother with handling their connection
			}

			id := -1
			for i := 0; i <= maxID; i++ {
				if !h.id[i] {
					id = i
					break
				}
			}
			if id == -1 {
				writeErrLog(conn.Ip, h.roomName, "room is full")
				continue
			}

			key := randstr.String(8)

			//sprite index < 0 means none
			client := &Client{
				hub:         h,
				conn:        conn.Connect,
				ip:          conn.Ip,
				send:        make(chan []byte, 256),
				id:          id,
				account:     isLoggedIn,
				name:        name,
				uuid:        uuid,
				rank:        rank,
				badge:       badge,
				spriteIndex: -1,
				tone:        [4]int{128, 128, 128, 128},
				pictures:    make(map[int]*Picture),
				mapId:       "0000",
				switchCache: make(map[int]bool),
				varCache:    make(map[int]int),
				key:         key}
			go client.writePump()
			go client.readPump()

			mapIdInt, errconv := strconv.Atoi(h.roomName)
			if errconv == nil {
				client.mapId = fmt.Sprintf("%04d", mapIdInt)
			}

			var isLoggedInBin int
			if isLoggedIn {
				isLoggedInBin = 1
			}

			tags, err := readPlayerTags(uuid)
			if err != nil {
				writeErrLog(conn.Ip, h.roomName, "failed to read player tags")
			} else {
				client.tags = tags
			}

			client.send <- []byte("s" + paramDelimStr + strconv.Itoa(id) + paramDelimStr + key + paramDelimStr + uuid + paramDelimStr + strconv.Itoa(rank) + paramDelimStr + strconv.Itoa(isLoggedInBin) + paramDelimStr + badge) //"your id is %id%" message

			//send the new client info about the game state
			for otherClient := range h.clients {
				var accountBin int
				if otherClient.account {
					accountBin = 1
				}
				client.send <- []byte("c" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + otherClient.uuid + paramDelimStr + strconv.Itoa(otherClient.rank) + paramDelimStr + strconv.Itoa(accountBin) + paramDelimStr + otherClient.badge)
				client.send <- []byte("m" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + strconv.Itoa(otherClient.x) + paramDelimStr + strconv.Itoa(otherClient.y))
				client.send <- []byte("f" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + strconv.Itoa(otherClient.facing))
				client.send <- []byte("spd" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + strconv.Itoa(otherClient.spd))
				if otherClient.name != "" {
					client.send <- []byte("name" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + otherClient.name)
				}
				if otherClient.spriteIndex >= 0 { //if the other client sent us valid sprite and index before
					client.send <- []byte("spr" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + otherClient.spriteName + paramDelimStr + strconv.Itoa(otherClient.spriteIndex))
				}
				if otherClient.repeatingFlash {
					client.send <- []byte("rfl" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + strconv.Itoa(otherClient.flash[0]) + paramDelimStr + strconv.Itoa(otherClient.flash[1]) + paramDelimStr + strconv.Itoa(otherClient.flash[2]) + paramDelimStr + strconv.Itoa(otherClient.flash[3]) + paramDelimStr + strconv.Itoa(otherClient.flash[4]))
				}
				if otherClient.tone[0] != 128 || otherClient.tone[1] != 128 || otherClient.tone[2] != 128 || otherClient.tone[3] != 128 {
					client.send <- []byte("t" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + strconv.Itoa(otherClient.tone[0]) + paramDelimStr + strconv.Itoa(otherClient.tone[1]) + paramDelimStr + strconv.Itoa(otherClient.tone[2]) + paramDelimStr + strconv.Itoa(otherClient.tone[3]))
				}
				if otherClient.systemName != "" {
					client.send <- []byte("sys" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + otherClient.systemName)
				}
				for picId, pic := range otherClient.pictures {
					var useTransparentColorBin int
					if pic.useTransparentColor {
						useTransparentColorBin = 1
					}
					var fixedToMapBin int
					if pic.fixedToMap {
						fixedToMapBin = 1
					}
					client.send <- []byte("ap" + paramDelimStr + strconv.Itoa(otherClient.id) + paramDelimStr + strconv.Itoa(picId) + paramDelimStr + strconv.Itoa(pic.positionX) + paramDelimStr + strconv.Itoa(pic.positionY) + paramDelimStr + strconv.Itoa(pic.mapX) + paramDelimStr + strconv.Itoa(pic.mapY) + paramDelimStr + strconv.Itoa(pic.panX) + paramDelimStr + strconv.Itoa(pic.panY) + paramDelimStr + strconv.Itoa(pic.magnify) + paramDelimStr + strconv.Itoa(pic.topTrans) + paramDelimStr + strconv.Itoa(pic.bottomTrans) + paramDelimStr + strconv.Itoa(pic.red) + paramDelimStr + strconv.Itoa(pic.blue) + paramDelimStr + strconv.Itoa(pic.green) + paramDelimStr + strconv.Itoa(pic.saturation) + paramDelimStr + strconv.Itoa(pic.effectMode) + paramDelimStr + strconv.Itoa(pic.effectPower) + paramDelimStr + pic.name + paramDelimStr + strconv.Itoa(useTransparentColorBin) + paramDelimStr + strconv.Itoa(fixedToMapBin))
				}
			}
			//register client in the structures
			h.id[id] = true
			h.clients[client] = true
			allClients[uuid] = client

			//tell everyone that a new client has connected
			h.broadcast([]byte("c" + paramDelimStr + strconv.Itoa(id) + paramDelimStr + uuid + paramDelimStr + strconv.Itoa(rank) + paramDelimStr + strconv.Itoa(isLoggedInBin) + paramDelimStr + badge)) //user %id% has connected message

			checkHubConditions(h, client, "", "")

			for _, minigame := range h.minigameConfigs {
				score, err := readPlayerMinigameScore(uuid, minigame.MinigameId)
				if err != nil {
					writeErrLog(conn.Ip, h.roomName, "failed to read player minigame score for "+minigame.MinigameId)
				}
				client.minigameScores = append(client.minigameScores, score)
				client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(minigame.VarId) + paramDelimStr + "1")
			}

			//send account-specific data like username
			if isLoggedIn {
				h.broadcast([]byte("name" + paramDelimStr + strconv.Itoa(id) + paramDelimStr + name)) //send name of client with account
			}

			writeLog(conn.Ip, h.roomName, "connect", 200)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				h.deleteClient(client)
			}

			writeLog(client.ip, h.roomName, "disconnect", 200)
		case message := <-h.processMsgCh:
			errs := h.processMsgs(message)
			if len(errs) > 0 {
				for _, err := range errs {
					writeErrLog(message.sender.ip, h.roomName, err.Error())
				}
			}
		}
	}
}

// serveWs handles websocket requests from the peer.
func (hub *Hub) serveWs(w http.ResponseWriter, r *http.Request) {
	protocols := r.Header.Get("Sec-Websocket-Protocol")
	conn, err := upgrader.Upgrade(w, r, http.Header{"Sec-Websocket-Protocol": {protocols}})
	if err != nil {
		log.Println(err)
		return
	}

	var playerSession string
	session, ok := r.URL.Query()["token"]
	if ok && len(session[0]) == 32 {
		playerSession = session[0]
	}

	hub.connect <- &ConnInfo{Connect: conn, Ip: getIp(r), Session: playerSession}
}

func (h *Hub) broadcast(data []byte) {
	if !h.singleplayer {
		for client := range h.clients {
			select {
			case client.send <- data:
			default:
				h.deleteClient(client)
			}
		}
	}
}

func (h *Hub) deleteClient(client *Client) {
	updatePlayerGameData(client) //update database
	delete(h.id, client.id)
	close(client.send)
	delete(h.clients, client)
	delete(allClients, client.uuid)
	h.broadcast([]byte("d" + paramDelimStr + strconv.Itoa(client.id))) //user %id% has disconnected message
}

func (h *Hub) processMsgs(msg *Message) []error {
	var errs []error

	if len(msg.data) < 12 || len(msg.data) > 4096 {
		errs = append(errs, errors.New("bad request size"))
		return errs
	}

	for _, v := range msg.data {
		if v < 32 {
			errs = append(errs, errors.New("bad byte sequence"))
			return errs
		}
	}

	if !utf8.Valid(msg.data) {
		errs = append(errs, errors.New("invalid UTF-8"))
		return errs
	}

	//signature validation
	byteKey := []byte(msg.sender.key)
	byteSecret := []byte(config.signKey)

	hashStr := sha1.New()
	hashStr.Write(byteKey)
	hashStr.Write(byteSecret)
	hashStr.Write(msg.data[8:])

	hashDigestStr := hex.EncodeToString(hashStr.Sum(nil)[:4])

	if string(msg.data[:8]) != hashDigestStr {
		//errs = append(errs, errors.New("bad signature"))
		errs = append(errs, errors.New("SIGNATURE FAIL: "+string(msg.data[:8])+" compared to "+hashDigestStr+" CONTENTS: "+string(msg.data[8:])))
		return errs
	}

	//counter validation
	playerMsgIndex, errconv := strconv.Atoi(string(msg.data[8:14]))
	if errconv != nil {
		errs = append(errs, errors.New("counter not numerical"))
		return errs
	}

	if msg.sender.counter < playerMsgIndex { //counter in messages should be higher than what we have stored
		msg.sender.counter = playerMsgIndex
	} else {
		errs = append(errs, errors.New("COUNTER FAIL: "+string(msg.data[8:14])+" compared to "+strconv.Itoa(msg.sender.counter)+" CONTENTS: "+string(msg.data[14:])))
		return errs
	}

	//message processing
	msgs := strings.Split(string(msg.data[14:]), msgDelimStr)

	for _, msgStr := range msgs {
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
	msgFields := strings.Split(msgStr, paramDelimStr)

	if len(msgFields) == 0 {
		return false, err
	}

	var terminate bool

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
		err = h.handleRrfl(msgFields, sender)
	case "t": //change my tone
		err = h.handleT(msgFields, sender)
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
		fallthrough
	case "gsay": //global say
		fallthrough
	case "psay": //party say
		err = h.handleSay(msgFields, sender)
		terminate = true
	case "name": // nick set
		err = h.handleName(msgFields, sender)
		terminate = true
	case "ss": // sync switch
		err = h.handleSs(msgFields, sender)
	case "sv": // sync variable
		err = h.handleSv(msgFields, sender)
	case "sev":
		err = h.handleSev(msgFields, sender)
	default:
		return false, err
	}

	if err != nil {
		return false, err
	}

	writeLog(sender.ip, h.roomName, msgStr, 200)

	return terminate, nil
}
