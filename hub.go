package main

import (
	"crypto/sha1"
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
	maxID  = 512
	delim  = "\uffff"
	mdelim = "\ufffe"
)

var (
	hubClients = make(map[string]*Client)
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	isOkString = regexp.MustCompile("^[A-Za-z0-9]+$").MatchString

	hubs []*Hub

	config         Config
	conditions     map[string]map[string]*Condition
	badges         map[string]map[string]*Badge
	sortedBadgeIds map[string][]string
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
	clients map[*Client]bool

	// True if the id is in use
	id map[int]bool

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
		processMsgCh:    make(chan *Message),
		connect:         make(chan *ConnInfo),
		unregister:      make(chan *Client),
		clients:         make(map[*Client]bool),
		id:              make(map[int]bool),
		roomId:          roomId,
		singleplayer:    singleplayer,
		conditions:      getHubConditions(roomId),
		minigameConfigs: getHubMinigameConfigs(roomId),
	}
}

func (h *Hub) run() {
	http.HandleFunc("/"+strconv.Itoa(h.roomId), h.serveWs)
	for {
		select {
		case conn := <-h.connect:
			uuid, _, _, _, _, banned, _ := getPlayerInfo(conn)
			if banned {
				writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "player is banned")
				continue
			}

			var session *SessionClient
			if s, ok := sessionClients[uuid]; ok {
				session = s
			} else {
				writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "player has no session")
				continue
			}

			var same_ip int
			for otherClient := range h.clients {
				if otherClient.session.ip == conn.Ip {
					same_ip++
				}
			}
			if same_ip >= 3 {
				writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "too many connections")
				continue //don't bother with handling their connection
			}

			var id int
			for i := 1; i <= maxID; i++ {
				if !h.id[i] {
					id = i
					break
				}
			}
			if id == 0 {
				writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "room is full")
				continue
			}

			key := randstr.String(8)

			//sprite index < 0 means none
			client := &Client{
				hub:         h,
				conn:        conn.Connect,
				send:        make(chan []byte, 256),
				session:     session,
				id:          id,
				key:         key,
				pictures:    make(map[int]*Picture),
				mapId:       fmt.Sprintf("%04d", h.roomId),
				switchCache: make(map[int]bool),
				varCache:    make(map[int]int),
			}
			go client.writePump()
			go client.readPump()

			tags, err := readPlayerTags(uuid)
			if err != nil {
				writeErrLog(conn.Ip, strconv.Itoa(h.roomId), "failed to read player tags")
			} else {
				client.tags = tags
			}

			client.send <- []byte("s" + delim + strconv.Itoa(id) + delim + key + delim + uuid + delim + strconv.Itoa(session.rank) + delim + btoa(session.account) + delim + session.badge) //"your id is %id%" message

			//register client in the structures
			h.id[id] = true
			h.clients[client] = true
			hubClients[uuid] = client

			writeLog(conn.Ip, strconv.Itoa(h.roomId), "connect", 200)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				h.deleteClient(client)
				writeLog(client.session.ip, strconv.Itoa(h.roomId), "disconnect", 200)
				continue
			}

			writeErrLog(client.session.ip, strconv.Itoa(h.roomId), "attempted to unregister nil client")
		case message := <-h.processMsgCh:
			errs := h.processMsgs(message)
			if len(errs) > 0 {
				for _, err := range errs {
					writeErrLog(message.sender.session.ip, strconv.Itoa(h.roomId), err.Error())
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

	var playerToken string
	token, ok := r.URL.Query()["token"]
	if ok && len(token[0]) == 32 {
		playerToken = token[0]
	}

	hub.connect <- &ConnInfo{Connect: conn, Ip: getIp(r), Token: playerToken}
}

func (h *Hub) broadcast(data []byte) {
	if h.singleplayer {
		return
	}

	for client := range h.clients {
		if !client.valid {
			continue
		}

		select {
		case client.send <- data:
		default:
			h.deleteClient(client)
		}
	}
}

func (h *Hub) deleteClient(client *Client) {
	delete(h.id, client.id)
	delete(h.clients, client)
	delete(hubClients, client.session.uuid)
	close(client.send)
	h.broadcast([]byte("d" + delim + strconv.Itoa(client.id))) //user %id% has disconnected message
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
	msgs := strings.Split(string(msg.data[14:]), mdelim)

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
			err = h.handleRrfl(msgFields, sender)
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
			return false, err
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

		//send account-specific data like username
		if client.session.account {
			h.broadcast([]byte("name" + delim + strconv.Itoa(client.id) + delim + client.session.name)) //send name of client with account
		}

		//send the new client info about the game state
		for otherClient := range h.clients {
			if !otherClient.valid {
				continue
			}

			var accountBin int
			if otherClient.session.account {
				accountBin = 1
			}
			client.send <- []byte("c" + delim + strconv.Itoa(otherClient.id) + delim + otherClient.session.uuid + delim + strconv.Itoa(otherClient.session.rank) + delim + strconv.Itoa(accountBin) + delim + otherClient.session.badge)
			client.send <- []byte("m" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(otherClient.x) + delim + strconv.Itoa(otherClient.y))
			client.send <- []byte("f" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(otherClient.facing))
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
				var useTransparentColorBin int
				if pic.useTransparentColor {
					useTransparentColorBin = 1
				}
				var fixedToMapBin int
				if pic.fixedToMap {
					fixedToMapBin = 1
				}
				client.send <- []byte("ap" + delim + strconv.Itoa(otherClient.id) + delim + strconv.Itoa(picId) + delim + strconv.Itoa(pic.positionX) + delim + strconv.Itoa(pic.positionY) + delim + strconv.Itoa(pic.mapX) + delim + strconv.Itoa(pic.mapY) + delim + strconv.Itoa(pic.panX) + delim + strconv.Itoa(pic.panY) + delim + strconv.Itoa(pic.magnify) + delim + strconv.Itoa(pic.topTrans) + delim + strconv.Itoa(pic.bottomTrans) + delim + strconv.Itoa(pic.red) + delim + strconv.Itoa(pic.blue) + delim + strconv.Itoa(pic.green) + delim + strconv.Itoa(pic.saturation) + delim + strconv.Itoa(pic.effectMode) + delim + strconv.Itoa(pic.effectPower) + delim + pic.name + delim + strconv.Itoa(useTransparentColorBin) + delim + strconv.Itoa(fixedToMapBin))
			}
		}
	}

	checkHubConditions(h, client, "", "")

	for _, minigame := range h.minigameConfigs {
		score, err := readPlayerMinigameScore(client.session.uuid, minigame.MinigameId)
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
}
