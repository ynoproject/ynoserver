// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"net/http"
	"log"
	"strconv"
	"strings"
	"regexp"
	"errors"
	"unicode"
	"unicode/utf8"
	"crypto/sha1"
	"encoding/hex"
	"github.com/thanhpk/randstr"
	"golang.org/x/text/unicode/norm"

	"github.com/gorilla/websocket"
)

var (
	maxID = 512
	totalPlayerCount = 0
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	isOkName = regexp.MustCompile("^[A-Za-z0-9]+$").MatchString
	paramDelimStr = "\uffff"
	msgDelimStr = "\ufffe"
)

type ConnInfo struct {
	Connect *websocket.Conn
	Ip string
}

type HubController struct {
	hubs []*Hub
}

func (h *HubController) addHub(roomName string, spriteNames []string, systemNames []string, soundNames []string, ignoredSoundNames []string, gameName string) {
	hub := NewHub(roomName, spriteNames, systemNames, soundNames, ignoredSoundNames, gameName, h)
	h.hubs = append(h.hubs, hub)
	go hub.Run()
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
	//list of valid game character sprite resource keys
	spriteNames []string
	systemNames []string
	soundNames []string
	ignoredSoundNames []string

	gameName string

	controller *HubController
}

func writeLog(ip string, roomName string, payload string, errorcode int) {
	log.Printf("%v %v \"%v\" %v\n", ip, roomName, strings.Replace(payload, "\"", "'", -1), errorcode)
}

func writeErrLog(ip string, roomName string, payload string) {
	writeLog(ip, roomName, payload, 400)
}

func CreateAllHubs(roomNames, spriteNames []string, systemNames []string, soundNames []string, ignoredSoundNames []string, gameName string) {
	h := HubController{}

	for _, roomName := range roomNames {
		h.addHub(roomName, spriteNames, systemNames, soundNames, ignoredSoundNames, gameName)
	}
}

func NewHub(roomName string, spriteNames []string, systemNames []string, soundNames []string, ignoredSoundNames []string, gameName string, h *HubController) *Hub {
	return &Hub{
		processMsgCh:  make(chan *Message),
		connect:   make(chan *ConnInfo),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		id: make(map[int]bool),
		roomName: roomName,
		spriteNames: spriteNames,
		systemNames: systemNames,
		soundNames: soundNames,
		ignoredSoundNames: ignoredSoundNames,
		gameName: gameName,
		controller: h,
	}
}

func (h *Hub) Run() {
	http.HandleFunc("/" + h.roomName, h.serveWs)
	for {
		select {
		case conn := <-h.connect:
			id := -1
			for i := 0; i <= maxID; i++ {
				if !h.id[i] {
					id = i
					break
				}
			}

			ip_limit := 3
			same_ip := 0
			for other_client := range h.clients {
				if other_client.ip == conn.Ip {
					same_ip++
				}
			}

			key := randstr.String(8)			

			//sprite index < 0 means none
			client := &Client{
				hub: h,
				conn: conn.Connect,
				ip: conn.Ip,
				banned: same_ip >= ip_limit,
				send: make(chan []byte, 256),
				id: id,
				x: 0,
				y: 0,
				facing: 0,
				name: "",
				spd: 3,
				spriteName: "none",
				spriteIndex: -1,
				key: key,
				counter: 0,
				}
			go client.writePump()
			go client.readPump()

			if same_ip >= ip_limit {
				writeErrLog(conn.Ip, h.roomName, "too many connections")
				close(client.send)
				continue
			}

			if id == -1 {
				writeErrLog(conn.Ip, h.roomName, "room is full")
				close(client.send)
				continue
			}

			totalPlayerCount = totalPlayerCount + 1

			client.send <- []byte("s" + paramDelimStr + strconv.Itoa(id) + paramDelimStr + strconv.Itoa(totalPlayerCount) + paramDelimStr + key) //"your id is %id%" (and player count) message
			//send the new client info about the game state
			for other_client := range h.clients {
				client.send <- []byte("c" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(totalPlayerCount))
				client.send <- []byte("m" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(other_client.x) + paramDelimStr + strconv.Itoa(other_client.y));
				client.send <- []byte("f" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(other_client.facing));
				client.send <- []byte("spd" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(other_client.spd));
				if other_client.name != "" {
					client.send <- []byte("name" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + other_client.name);
				}
				if other_client.spriteIndex >= 0 { //if the other client sent us valid sprite and index before
					client.send <- []byte("spr" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + other_client.spriteName + paramDelimStr + strconv.Itoa(other_client.spriteIndex));
				}
				if other_client.systemName != "" {
					client.send <- []byte("sys" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + other_client.systemName);
				}
			}
			//register client in the structures
			h.id[id] = true
			h.clients[client] = true
			//tell everyone that a new client has connected
			if !client.banned {
				h.broadcast([]byte("c" + paramDelimStr + strconv.Itoa(id) + paramDelimStr + strconv.Itoa(totalPlayerCount))) //user %id% has connected (and player count) message
			}

			writeLog(conn.Ip, h.roomName, "connect", 200)
		case client := <-h.unregister:
			totalPlayerCount = totalPlayerCount - 1

			if _, ok := h.clients[client]; ok {
				h.deleteClient(client)
			}

			writeLog(client.ip, h.roomName, "disconnect", 200)
		case message := <-h.processMsgCh:
			errs := h.processMsg(message)
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
	ip := r.Header.Get("x-forwarded-for")
	if ip == "" {
		ip = r.RemoteAddr
	}
	hub.connect <- &ConnInfo{Connect: conn, Ip: ip}
}

func (h *Hub) broadcast(data []byte) {
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			h.deleteClient(client)
		}
	}
}

func (h *Hub) deleteClient(client *Client) {
	delete(h.id, client.id)
	close(client.send)
	delete(h.clients, client)
	h.broadcast([]byte("d" + paramDelimStr + strconv.Itoa(client.id) + paramDelimStr + strconv.Itoa(totalPlayerCount))) //user %id% has disconnected (and new player count) message
}

func (h *Hub) processMsg(msg *Message) []error {
	var errs []error

	if msg.sender.banned {
		errs = append(errs, errors.New("banned"))
		return errs
	}

	if len(msg.data) < 12 || len(msg.data) > 512 {
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
	byteSecret := []byte("")

	hashStr := sha1.New()
	hashStr.Write(byteKey)
	hashStr.Write(byteSecret)
	hashStr.Write(msg.data[8:])

	hashDigestStr := hex.EncodeToString(hashStr.Sum(nil)[:4])
	
	if string(msg.data[:8]) != hashDigestStr {
		//errs = append(errs, errors.New("bad signature"))
		errs = append(errs, errors.New("SIGNATURE FAIL: " + string(msg.data[:8]) + " compared to " + hashDigestStr + " CONTENTS: " + string(msg.data[8:])))
		return errs
	}

	//counter validation
	playerMsgIndex, errconv := strconv.Atoi(string(msg.data[8:14]))
	if errconv != nil {
		//errs = append(errs, errors.New("counter not numerical"))
		errs = append(errs, errors.New("COUNTER FAIL: " + string(msg.data[8:14]) + " compared to " + strconv.Itoa(msg.sender.counter) + " CONTENTS: " + string(msg.data[14:])))
		return errs
	}

	if msg.sender.counter < playerMsgIndex  { //counter in messages should be higher than what we have stored
		msg.sender.counter = playerMsgIndex
	} else {
		errs = append(errs, errors.New("counter too low"))
		return errs
	}

	//message processing
	msgsStr := string(msg.data[14:])
	msgs := strings.Split(msgsStr, msgDelimStr)
	terminate := false
	
	for _, msgStr := range msgs {
		err := errors.New(msgStr)
		msgFields := strings.Split(msgStr, paramDelimStr)

		if len(msgFields) == 0 {
			errs = append(errs, err)
			continue
		}

		switch msgFields[0] {
		case "m": //"i moved to x y"
			if len(msgFields) != 3 {
				errs = append(errs, err)
				continue
			}
			//check if the coordinates are valid
			x, errconv := strconv.Atoi(msgFields[1])
			if errconv != nil {
				errs = append(errs, err)
				continue
			}
			y, errconv := strconv.Atoi(msgFields[2]);
			if errconv != nil {
				errs = append(errs, err)
				continue
			}
			msg.sender.x = x
			msg.sender.y = y
			h.broadcast([]byte("m" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2])) //user %id% moved to x y
		case "f": //change facing direction
			if len(msgFields) != 2 {
				errs = append(errs, err)
				continue
			}
			//check if direction is valid
			facing, errconv := strconv.Atoi(msgFields[1])
			if errconv != nil || facing < 0 || facing > 3 {
				errs = append(errs, err)
				continue
			}
			msg.sender.facing = facing
			h.broadcast([]byte("f" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1])) //user %id% facing changed to f
		case "spd": //change my speed to spd
			if len(msgFields) != 2 {
				errs = append(errs, err)
				continue
			}
			spd, errconv := strconv.Atoi(msgFields[1])
			if errconv != nil {
				errs = append(errs, err)
				continue
			}
			if spd < 0 || spd > 10 { //something's not right
				errs = append(errs, err)
				continue
			}
			msg.sender.spd = spd
			h.broadcast([]byte("spd" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1]));
		case "spr": //change my sprite
			if len(msgFields) != 3 {
				errs = append(errs, err)
				continue
			}
			if !h.isValidSpriteName(msgFields[1]) {
				errs = append(errs, err)
				continue
			}
			if h.gameName == "2kki" { //totally normal yume 2kki check
				if !strings.Contains(msgFields[1], "syujinkou") && !strings.Contains(msgFields[1], "effect") && !strings.Contains(msgFields[1], "yukihitsuji_game") && !strings.Contains(msgFields[1], "zenmaigaharaten_kisekae") {
					errs = append(errs, err)
					continue
				}
				if strings.Contains(msgFields[1], "zenmaigaharaten_kisekae") && h.roomName != "MAP0176 ぜんまいヶ原店"  {
					errs = append(errs, err)
					continue
				}
			}
			index, errconv := strconv.Atoi(msgFields[2])
			if errconv != nil || index < 0 {
				errs = append(errs, err)
				continue
			}
			msg.sender.spriteName = msgFields[1]
			msg.sender.spriteIndex = index
			h.broadcast([]byte("spr" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2]));
		case "sys": //change my system graphic
			if len(msgFields) != 2 {
				errs = append(errs, err)
				continue
			}
			if !h.isValidSystemName(msgFields[1]) {
				errs = append(errs, err)
				continue
			}
			msg.sender.systemName = msgFields[1];
			h.broadcast([]byte("sys" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1]));
		case "se": //play sound effect
			if len(msgFields) != 5 || msgFields[1] == "" {
				errs = append(errs, err)
				continue
			}
			if !h.isValidSoundName(msgFields[1]) {
				errs = append(errs, err)
				continue
			}
			volume, errconv := strconv.Atoi(msgFields[2])
			if errconv != nil || volume < 0 || volume > 100 {
				errs = append(errs, err)
				continue
			}
			tempo, errconv := strconv.Atoi(msgFields[3])
			if errconv != nil || tempo < 10 || tempo > 400 {
				errs = append(errs, err)
				continue
			}
			balance, errconv := strconv.Atoi(msgFields[4])
			if errconv != nil || balance < 0 || balance > 100 {
				errs = append(errs, err)
				continue
			}
			h.broadcast([]byte("se" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2] + paramDelimStr + msgFields[3] + paramDelimStr + msgFields[4]));
		case "ap": // picture shown
			fallthrough
		case "mp": // picture moved
			errs = append(errs, err) // temporarily disable this feature
			continue
			isShow := msgFields[0] == "ap"
			msgLength := 17
			if (isShow) {
				msgLength++
			}
			if len(msgFields) != msgLength {
				errs = append(errs, err)
				continue
			}
			picId, errconv := strconv.Atoi(msgFields[1])
			if errconv != nil || picId < 1 {
				errs = append(errs, err)
				continue
			}
			message := msgFields[0] + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2] + paramDelimStr + msgFields[3] + paramDelimStr + msgFields[4] + paramDelimStr + msgFields[5] + paramDelimStr + msgFields[6] + paramDelimStr + msgFields[7] + paramDelimStr + msgFields[8] + paramDelimStr + msgFields[9] + paramDelimStr + msgFields[10] + paramDelimStr + msgFields[11] + paramDelimStr + msgFields[12] + paramDelimStr + msgFields[13] + paramDelimStr + msgFields[14] + paramDelimStr + msgFields[15] + paramDelimStr + msgFields[16]
			if (isShow) {
				message += paramDelimStr + msgFields[17]
			}
			h.broadcast([]byte(message));
		case "rp": // picture erased
			errs = append(errs, err) // temporarily disable this feature
			continue
			if len(msgFields) != 2 {
				errs = append(errs, err)
				continue
			}
			picId, errconv := strconv.Atoi(msgFields[1])
			if errconv != nil || picId < 1 {
				errs = append(errs, err)
				continue
			}
			h.broadcast([]byte("rp" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msgFields[1]));
		case "say":
			if len(msgFields) != 3 {
				errs = append(errs, err)
				break
			}
			systemName := msgFields[1]
			msgContents := msgFields[2]
			if msg.sender.name == "" || !h.isValidSystemName(systemName) || msgContents == "" || len(msgContents) > 150 {
				errs = append(errs, err)
				break
			}
			h.broadcast([]byte("say" + paramDelimStr + systemName + paramDelimStr + "<" + msg.sender.name + "> " + msgContents));
			terminate = true
		case "name": // nick set
			if msg.sender.name != "" || len(msgFields) != 2 || !isOkName(msgFields[1]) || len(msgFields[1]) > 12 {
				errs = append(errs, err)
				break
			}
			msg.sender.name = msgFields[1]
			h.broadcast([]byte("name" + paramDelimStr + strconv.Itoa(msg.sender.id) + paramDelimStr + msg.sender.name));
			terminate = true
		default:
			errs = append(errs, err)
		}

		writeLog(msg.sender.ip, h.roomName, msgStr, 200)
		
		if terminate {
			break
		}
	}

	return errs
}

func normalize(str string) string {
	var (
		r   rune
		it  norm.Iter
		out []byte
	)
	it.InitString(norm.NFKC, str)
	for !it.Done() {
		ruf := it.Next()
		r, _ = utf8.DecodeRune(ruf)
		r = unicode.ToLower(r)
		buf := make([]byte, utf8.RuneLen(r))
		utf8.EncodeRune(buf, r)
		out = norm.NFC.Append(out, buf...)
	}
	return string(out)
}

func (h *Hub) isValidSpriteName(name string) bool {
	if name == "" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range h.spriteNames {
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func (h *Hub) isValidSystemName(name string) bool {
	for _, otherName := range h.systemNames {
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func (h *Hub) isValidSoundName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	
	for _, otherName := range h.soundNames {
		if strings.EqualFold(otherName, name) {
			for _, ignoredName := range h.ignoredSoundNames {
				if strings.EqualFold(ignoredName, name) {
					return false
				}
			}
		} else {
			return false
		}
	}
	return true
}
