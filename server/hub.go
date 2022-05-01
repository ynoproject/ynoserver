package server

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/thanhpk/randstr"
)

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

	conditions []*Condition
}

func (h *Hub) Run() {
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
				tone:        []int{128, 128, 128, 128},
				pictures:    make(map[int]*Picture),
				mapId:       "0000",
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
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			h.deleteClient(client)
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
	case "m": //"i moved to x y"
		if len(msgFields) != 3 {
			return false, err
		}
		//check if the coordinates are valid
		x, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil || x < 0 {
			return false, err
		}
		y, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil || y < 0 {
			return false, err
		}
		sender.x = x
		sender.y = y
		h.broadcast([]byte("m" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2])) //user %id% moved to x y
	case "f": //change facing direction
		if len(msgFields) != 2 {
			return false, err
		}
		//check if direction is valid
		facing, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil || facing < 0 || facing > 3 {
			return false, err
		}
		sender.facing = facing
		h.broadcast([]byte("f" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1])) //user %id% facing changed to f
	case "spd": //change my speed to spd
		if len(msgFields) != 2 {
			return false, err
		}
		spd, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil {
			return false, err
		}
		if spd < 0 || spd > 10 { //something's not right
			return false, err
		}
		sender.spd = spd
		h.broadcast([]byte("spd" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1]))
	case "spr": //change my sprite
		if len(msgFields) != 3 {
			return false, err
		}
		if !isValidSpriteName(msgFields[1]) {
			return false, err
		}
		if config.gameName == "2kki" { //totally normal yume 2kki check
			if !strings.Contains(msgFields[1], "syujinkou") && !strings.Contains(msgFields[1], "effect") && !strings.Contains(msgFields[1], "yukihitsuji_game") && !strings.Contains(msgFields[1], "zenmaigaharaten_kisekae") {
				return false, err
			}
			if strings.Contains(msgFields[1], "zenmaigaharaten_kisekae") && h.roomName != "176" {
				return false, err
			}
		}
		index, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil || index < 0 {
			return false, err
		}
		sender.spriteName = msgFields[1]
		sender.spriteIndex = index
		h.broadcast([]byte("spr" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2]))
	case "fl": //player flash
		fallthrough
	case "rfl": //repeating player flash
		if len(msgFields) != 6 {
			return false, err
		}
		red, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil || red < 0 || red > 255 {
			return false, err
		}
		green, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil || green < 0 || green > 255 {
			return false, err
		}
		blue, errconv := strconv.Atoi(msgFields[3])
		if errconv != nil || blue < 0 || blue > 255 {
			return false, err
		}
		power, errconv := strconv.Atoi(msgFields[4])
		if errconv != nil || power < 0 {
			return false, err
		}
		frames, errconv := strconv.Atoi(msgFields[5])
		if errconv != nil || frames < 0 {
			return false, err
		}
		if msgFields[0] == "rfl" {
			sender.flash[0] = red
			sender.flash[1] = green
			sender.flash[2] = blue
			sender.flash[3] = power
			sender.flash[4] = frames
			sender.repeatingFlash = true
		}
		h.broadcast([]byte(msgFields[0] + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2] + paramDelimStr + msgFields[3] + paramDelimStr + msgFields[4] + paramDelimStr + msgFields[5]))
	case "rrfl": //remove repeating player flash
		sender.repeatingFlash = false
		for i := 0; i < 5; i++ {
			sender.flash[i] = 0
		}
		h.broadcast([]byte("rrfl" + paramDelimStr + strconv.Itoa(sender.id)))
	case "t": //change my tone
		if len(msgFields) != 5 {
			return false, err
		}
		red, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil || red < 0 || red > 255 {
			return false, err
		}
		green, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil || green < 0 || green > 255 {
			return false, err
		}
		blue, errconv := strconv.Atoi(msgFields[3])
		if errconv != nil || blue < 0 || blue > 255 {
			return false, err
		}
		gray, errconv := strconv.Atoi(msgFields[4])
		if errconv != nil || red < 0 || gray > 255 {
			return false, err
		}
		sender.tone[0] = red
		sender.tone[1] = green
		sender.tone[2] = blue
		sender.tone[3] = gray
		h.broadcast([]byte("t" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2] + paramDelimStr + msgFields[3] + paramDelimStr + msgFields[4]))
	case "sys": //change my system graphic
		if len(msgFields) != 2 {
			return false, err
		}
		if !isValidSystemName(msgFields[1], false) {
			return false, err
		}
		sender.systemName = msgFields[1]
		h.broadcast([]byte("sys" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1]))
	case "se": //play sound effect
		if len(msgFields) != 5 || msgFields[1] == "" {
			return false, err
		}
		if !isValidSoundName(msgFields[1]) {
			return false, err
		}
		volume, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil || volume < 0 || volume > 100 {
			return false, err
		}
		tempo, errconv := strconv.Atoi(msgFields[3])
		if errconv != nil || tempo < 10 || tempo > 400 {
			return false, err
		}
		balance, errconv := strconv.Atoi(msgFields[4])
		if errconv != nil || balance < 0 || balance > 100 {
			return false, err
		}
		h.broadcast([]byte("se" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2] + paramDelimStr + msgFields[3] + paramDelimStr + msgFields[4]))
	case "ap": // picture shown
		fallthrough
	case "mp": // picture moved
		isShow := msgFields[0] == "ap"
		msgLength := 18
		if isShow {
			msgLength = msgLength + 2
		}
		if len(msgFields) != msgLength {
			return false, err
		}

		if isShow {
			checkHubConditions(h, sender, "picture", msgFields[17])
			if !isValidPicName(msgFields[17]) {
				return false, err
			}
		}

		picId, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil || picId < 1 {
			return false, err
		}

		positionX, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil {
			return false, err
		}
		positionY, errconv := strconv.Atoi(msgFields[3])
		if errconv != nil {
			return false, err
		}
		mapX, errconv := strconv.Atoi(msgFields[4])
		if errconv != nil {
			return false, err
		}
		mapY, errconv := strconv.Atoi(msgFields[5])
		if errconv != nil {
			return false, err
		}
		panX, errconv := strconv.Atoi(msgFields[6])
		if errconv != nil {
			return false, err
		}
		panY, errconv := strconv.Atoi(msgFields[7])
		if errconv != nil {
			return false, err
		}

		magnify, errconv := strconv.Atoi(msgFields[8])
		if errconv != nil || magnify < 0 {
			return false, err
		}
		topTrans, errconv := strconv.Atoi(msgFields[9])
		if errconv != nil || topTrans < 0 {
			return false, err
		}
		bottomTrans, errconv := strconv.Atoi(msgFields[10])
		if errconv != nil || bottomTrans < 0 {
			return false, err
		}

		red, errconv := strconv.Atoi(msgFields[11])
		if errconv != nil || red < 0 || red > 200 {
			return false, err
		}
		green, errconv := strconv.Atoi(msgFields[12])
		if errconv != nil || green < 0 || green > 200 {
			return false, err
		}
		blue, errconv := strconv.Atoi(msgFields[13])
		if errconv != nil || blue < 0 || blue > 200 {
			return false, err
		}
		saturation, errconv := strconv.Atoi(msgFields[14])
		if errconv != nil || saturation < 0 || saturation > 200 {
			return false, err
		}

		effectMode, errconv := strconv.Atoi(msgFields[15])
		if errconv != nil || effectMode < 0 {
			return false, err
		}
		effectPower, errconv := strconv.Atoi(msgFields[16])
		if errconv != nil {
			return false, err
		}

		var pic *Picture
		if isShow {
			picName := msgFields[17]
			if picName == "" {
				return false, err
			}

			useTransparentColorBin, errconv := strconv.Atoi(msgFields[18])
			if errconv != nil || useTransparentColorBin < 0 || useTransparentColorBin > 1 {
				return false, err
			}

			fixedToMapBin, errconv := strconv.Atoi(msgFields[19])
			if errconv != nil || fixedToMapBin < 0 || fixedToMapBin > 1 {
				return false, err
			}

			pic = &Picture{
				name:                picName,
				useTransparentColor: useTransparentColorBin == 1,
				fixedToMap:          fixedToMapBin == 1,
			}

			if _, found := sender.pictures[picId]; found {
				rpTerminate, rpErr := h.processMsg("rp"+paramDelimStr+msgFields[1], sender)
				if rpErr != nil {
					return rpTerminate, rpErr
				}
			}
		} else {
			if _, found := sender.pictures[picId]; found {
				duration, errconv := strconv.Atoi(msgFields[17])
				if errconv != nil || duration < 0 {
					return false, err
				}

				pic = sender.pictures[picId]
			} else {
				return false, nil
			}
		}

		pic.positionX = positionX
		pic.positionY = positionY
		pic.mapX = mapX
		pic.mapY = mapY
		pic.panX = panX
		pic.panY = panY
		pic.magnify = magnify
		pic.topTrans = topTrans
		pic.bottomTrans = bottomTrans
		pic.red = red
		pic.blue = blue
		pic.green = green
		pic.saturation = saturation
		pic.effectMode = effectMode
		pic.effectPower = effectPower

		message := msgFields[0] + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1] + paramDelimStr + msgFields[2] + paramDelimStr + msgFields[3] + paramDelimStr + msgFields[4] + paramDelimStr + msgFields[5] + paramDelimStr + msgFields[6] + paramDelimStr + msgFields[7] + paramDelimStr + msgFields[8] + paramDelimStr + msgFields[9] + paramDelimStr + msgFields[10] + paramDelimStr + msgFields[11] + paramDelimStr + msgFields[12] + paramDelimStr + msgFields[13] + paramDelimStr + msgFields[14] + paramDelimStr + msgFields[15] + paramDelimStr + msgFields[16] + paramDelimStr + msgFields[17]
		if isShow {
			message += paramDelimStr + msgFields[18] + paramDelimStr + msgFields[19]
		}
		h.broadcast([]byte(message))
		sender.pictures[picId] = pic
	case "rp": // picture erased
		if len(msgFields) != 2 {
			return false, err
		}
		picId, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil || picId < 1 {
			return false, err
		}
		h.broadcast([]byte("rp" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgFields[1]))
		delete(sender.pictures, picId)
	case "say":
		fallthrough
	case "gsay": //global say
		fallthrough
	case "psay": //party say
		msgLength := 2
		if msgFields[0] == "gsay" {
			msgLength++
		}
		if len(msgFields) != msgLength {
			return true, err
		}
		msgContents := msgFields[1]
		if sender.name == "" || sender.systemName == "" || msgContents == "" || len(msgContents) > 150 {
			return true, err
		}
		switch msgFields[0] {
		case "gsay":
			enableLocBin, errconv := strconv.Atoi(msgFields[2])
			if errconv != nil || enableLocBin < 0 || enableLocBin > 1 {
				return false, err
			}

			mapId := "0000"
			prevMapId := "0000"
			var prevLocations string
			x := -1
			y := -1

			if enableLocBin == 1 {
				mapId = sender.mapId
				prevMapId = sender.prevMapId
				prevLocations = sender.prevLocations
				x = sender.x
				y = sender.y
			}

			var accountBin int
			if sender.account {
				accountBin = 1
			}

			globalBroadcast([]byte("gsay" + paramDelimStr + sender.uuid + paramDelimStr + sender.name + paramDelimStr + sender.systemName + paramDelimStr + strconv.Itoa(sender.rank) + paramDelimStr + strconv.Itoa(accountBin) + paramDelimStr + sender.badge + paramDelimStr + mapId + paramDelimStr + prevMapId + paramDelimStr + prevLocations + paramDelimStr + strconv.Itoa(x) + paramDelimStr + strconv.Itoa(y) + paramDelimStr + msgContents))
		case "psay":
			partyId, err := readPlayerPartyId(sender.uuid)
			if err != nil {
				return true, err
			}
			if partyId == 0 {
				return true, errors.New("player not in a party")
			}
			partyMemberUuids, err := readPartyMemberUuids(partyId)
			if err != nil {
				return true, err
			}
			for _, uuid := range partyMemberUuids {
				if _, ok := allClients[uuid]; ok {
					allClients[uuid].send <- []byte("psay" + paramDelimStr + sender.uuid + paramDelimStr + msgContents)
				}
			}
		default:
			h.broadcast([]byte("say" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgContents))
		}
		terminate = true
	case "name": // nick set
		if sender.name != "" || len(msgFields) != 2 || !isOkString(msgFields[1]) || len(msgFields[1]) > 12 {
			return true, err
		}
		sender.name = msgFields[1]
		h.broadcast([]byte("name" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + sender.name))
		terminate = true
	case "ss": // sync switch
		switchId, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil {
			return false, err
		}
		valueBin, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil || valueBin < 0 || valueBin > 1 {
			return false, err
		}
		value := false
		if valueBin == 1 {
			value = true
		}
		for _, c := range h.conditions {
			if switchId == c.SwitchId {
				if value == c.SwitchValue {
					if !c.TimeTrial {
						if checkConditionCoords(c, sender) {
							success, err := tryWritePlayerTag(sender.uuid, c.ConditionId)
							if err != nil {
								return false, err
							}
							if success {
								sender.send <- []byte("b")
							}
						}
					} else if config.gameName == "2kki" {
						sender.send <- []byte("sv" + paramDelimStr + "88" + paramDelimStr + "0")
					}
				}
			} else if len(c.SwitchIds) > 0 {
				for s, sId := range c.SwitchIds {
					if switchId == sId {
						if value == c.SwitchValues[s] {
							if s == len(c.SwitchIds)-1 {
								if !c.TimeTrial {
									if checkConditionCoords(c, sender) {
										success, err := tryWritePlayerTag(sender.uuid, c.ConditionId)
										if err != nil {
											return false, err
										}
										if success {
											sender.send <- []byte("b")
										}
									}
								} else if config.gameName == "2kki" {
									sender.send <- []byte("sv" + paramDelimStr + "88" + paramDelimStr + "0")
								}
							} else {
								sender.send <- []byte("ss" + paramDelimStr + strconv.Itoa(c.SwitchIds[s+1]) + paramDelimStr + "0")
							}
						}
						break
					}
				}
			}
		}
	case "sv": // sync variable
		varId, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil {
			return false, err
		}
		value, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil {
			return false, err
		}
		if varId == 88 && config.gameName == "2kki" {
			switch varId {
			case 88:
				for _, c := range h.conditions {
					if c.TimeTrial && value < 3600 {
						if checkConditionCoords(c, sender) {
							mapId, _ := strconv.Atoi(h.roomName)
							success, err := tryWritePlayerTimeTrial(sender.uuid, mapId, value)
							if err != nil {
								return false, err
							}
							if success {
								sender.send <- []byte("b")
							}
						}
					}
				}
			}
		} else {
			for _, c := range h.conditions {
				if varId == c.VarId {
					valid := false
					switch c.VarOp {
					case "=":
						valid = value == c.VarValue
					case "<":
						valid = value < c.VarValue
					case ">":
						valid = value > c.VarValue
					case "<=":
						valid = value <= c.VarValue
					case ">=":
						valid = value >= c.VarValue
					case "!=":
						valid = value != c.VarValue
					}
					if valid {
						if !c.TimeTrial {
							if checkConditionCoords(c, sender) {
								success, err := tryWritePlayerTag(sender.uuid, c.ConditionId)
								if err != nil {
									return false, err
								}
								if success {
									sender.send <- []byte("b")
								}
							}
						} else if config.gameName == "2kki" {
							sender.send <- []byte("sv" + paramDelimStr + "88" + paramDelimStr + "0")
						}
					}
				} else if len(c.VarIds) > 0 {
					for v, vId := range c.VarIds {
						if varId == vId {
							valid := false
							switch c.VarOp {
							case "=":
								valid = value == c.VarValues[v]
							case "<":
								valid = value < c.VarValues[v]
							case ">":
								valid = value > c.VarValues[v]
							case "<=":
								valid = value <= c.VarValues[v]
							case ">=":
								valid = value >= c.VarValues[v]
							case "!=":
								valid = value != c.VarValues[v]
							}
							if valid {
								if v == len(c.VarIds)-1 {
									if !c.TimeTrial {
										if checkConditionCoords(c, sender) {
											success, err := tryWritePlayerTag(sender.uuid, c.ConditionId)
											if err != nil {
												return false, err
											}
											if success {
												sender.send <- []byte("b")
											}
										}
									} else if config.gameName == "2kki" {
										sender.send <- []byte("sv" + paramDelimStr + "88" + paramDelimStr + "0")
									}
								} else {
									sender.send <- []byte("sv" + paramDelimStr + strconv.Itoa(c.VarIds[v+1]) + paramDelimStr + "0")
								}
							}
							break
						}
					}
				}
			}
		}
	default:
		return false, err
	}

	writeLog(sender.ip, h.roomName, msgStr, 200)

	return terminate, nil
}
