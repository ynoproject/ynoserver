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
}

func (h *Hub) Run() {
	http.HandleFunc("/"+h.roomName, h.serveWs)
	for {
		select {
		case conn := <-h.connect:
			uuid, rank, banned := readPlayerData(conn.Ip)
			if banned {
				writeErrLog(conn.Ip, h.roomName, "user is banned")
				continue
			}

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
			if same_ip >= ip_limit {
				writeErrLog(conn.Ip, h.roomName, "too many connections")
				continue //don't bother with handling their connection
			}

			key := randstr.String(8)

			//sprite index < 0 means none
			client := &Client{
				hub:         h,
				conn:        conn.Connect,
				ip:          conn.Ip,
				send:        make(chan []byte, 256),
				id:          id,
				uuid:        uuid,
				rank:        rank,
				spriteIndex: -1,
				pictures:    make(map[int]*Picture),
				key:         key}
			go client.writePump()
			go client.readPump()

			if id == -1 {
				writeErrLog(conn.Ip, h.roomName, "room is full")
				close(client.send)
				continue
			}

			client.send <- []byte("s" + paramDelimStr + strconv.Itoa(id) + paramDelimStr + key + paramDelimStr + uuid + paramDelimStr + strconv.Itoa(rank)) //"your id is %id%" message
			//send the new client info about the game state
			for other_client := range h.clients {
				client.send <- []byte("c" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + other_client.uuid + paramDelimStr + strconv.Itoa(other_client.rank))
				client.send <- []byte("m" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(other_client.x) + paramDelimStr + strconv.Itoa(other_client.y))
				client.send <- []byte("f" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(other_client.facing))
				client.send <- []byte("spd" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(other_client.spd))
				if other_client.name != "" {
					client.send <- []byte("name" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + other_client.name)
				}
				if other_client.spriteIndex >= 0 { //if the other client sent us valid sprite and index before
					client.send <- []byte("spr" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + other_client.spriteName + paramDelimStr + strconv.Itoa(other_client.spriteIndex))
				}
				if other_client.systemName != "" {
					client.send <- []byte("sys" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + other_client.systemName)
				}
				for picId, pic := range other_client.pictures {
					useTransparentColorBin := 0
					if pic.useTransparentColor {
						useTransparentColorBin = 1
					}
					fixedToMapBin := 0
					if pic.fixedToMap {
						fixedToMapBin = 1
					}
					client.send <- []byte("ap" + paramDelimStr + strconv.Itoa(other_client.id) + paramDelimStr + strconv.Itoa(picId) + paramDelimStr + strconv.Itoa(pic.positionX) + paramDelimStr + strconv.Itoa(pic.positionY) + paramDelimStr + strconv.Itoa(pic.mapX) + paramDelimStr + strconv.Itoa(pic.mapY) + paramDelimStr + strconv.Itoa(pic.panX) + paramDelimStr + strconv.Itoa(pic.panY) + paramDelimStr + strconv.Itoa(pic.magnify) + paramDelimStr + strconv.Itoa(pic.topTrans) + paramDelimStr + strconv.Itoa(pic.bottomTrans) + paramDelimStr + strconv.Itoa(pic.red) + paramDelimStr + strconv.Itoa(pic.blue) + paramDelimStr + strconv.Itoa(pic.green) + paramDelimStr + strconv.Itoa(pic.saturation) + paramDelimStr + strconv.Itoa(pic.effectMode) + paramDelimStr + strconv.Itoa(pic.effectPower) + paramDelimStr + pic.name + paramDelimStr + strconv.Itoa(useTransparentColorBin) + paramDelimStr + strconv.Itoa(fixedToMapBin))
				}
			}
			//register client in the structures
			h.id[id] = true
			h.clients[client] = true
			allClients[uuid] = client

			//tell everyone that a new client has connected
			h.broadcast([]byte("c" + paramDelimStr + strconv.Itoa(id) + paramDelimStr + uuid + paramDelimStr + strconv.Itoa(rank))) //user %id% has connected message

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
	updatePlayerData(client) //update database
	delete(h.id, client.id)
	close(client.send)
	delete(h.clients, client)
	delete(allClients, client.uuid)
	h.broadcast([]byte("d" + paramDelimStr + strconv.Itoa(client.id))) //user %id% has disconnected (and new player count) message
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
		//errs = append(errs, errors.New("counter not numerical"))
		errs = append(errs, errors.New("COUNTER FAIL: "+string(msg.data[8:14])+" compared to "+strconv.Itoa(msg.sender.counter)+" CONTENTS: "+string(msg.data[14:])))
		return errs
	}

	if msg.sender.counter < playerMsgIndex { //counter in messages should be higher than what we have stored
		msg.sender.counter = playerMsgIndex
	} else {
		errs = append(errs, errors.New("counter too low"))
		return errs
	}

	//message processing
	msgsStr := string(msg.data[14:])
	msgs := strings.Split(msgsStr, msgDelimStr)

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

	terminate := false

	switch msgFields[0] {
	case "m": //"i moved to x y"
		if len(msgFields) != 3 {
			return false, err
		}
		//check if the coordinates are valid
		x, errconv := strconv.Atoi(msgFields[1])
		if errconv != nil {
			return false, err
		}
		y, errconv := strconv.Atoi(msgFields[2])
		if errconv != nil {
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
	case "sys": //change my system graphic
		if len(msgFields) != 2 {
			return false, err
		}
		if !isValidSystemName(msgFields[1]) {
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

		if isShow && !isValidPicName(msgFields[17]) {
			return false, err
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

			var newPic Picture

			newPic.name = picName
			newPic.useTransparentColor = useTransparentColorBin == 1
			newPic.fixedToMap = fixedToMapBin == 1
			pic = &newPic

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
		isGlobal := msgFields[0] == "gsay"
		msgLength := 2
		if isGlobal {
			msgLength++
		}
		if len(msgFields) != msgLength {
			return true, err
		}
		msgContents := msgFields[1]
		if sender.name == "" || sender.systemName == "" || msgContents == "" || len(msgContents) > 150 {
			return true, err
		}
		if !isGlobal {
			h.broadcast([]byte("say" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + msgContents))
		} else {
			enableLocBin, errconv := strconv.Atoi(msgFields[2])
			if errconv != nil || enableLocBin < 0 || enableLocBin > 1 {
				return false, err
			}

			mapId := "0000"
			prevMapId := "0000"
			prevLocations := ""

			if enableLocBin == 1 {
				mapIdInt, errconv := strconv.Atoi(h.roomName)
				if errconv != nil {
					return true, err
				}
				mapId = fmt.Sprintf("%04d", mapIdInt)
				prevMapId = sender.prevMapId
				prevLocations = sender.prevLocations
			}
			globalBroadcast([]byte("gsay" + paramDelimStr + sender.uuid + paramDelimStr + sender.name + paramDelimStr + sender.systemName + paramDelimStr + strconv.Itoa(sender.rank) + paramDelimStr + mapId + paramDelimStr + prevMapId + paramDelimStr + prevLocations + paramDelimStr + msgContents))
		}
		terminate = true
	case "name": // nick set
		if sender.name != "" || len(msgFields) != 2 || !isOkName(msgFields[1]) || len(msgFields[1]) > 12 {
			return true, err
		}
		sender.name = msgFields[1]
		h.broadcast([]byte("name" + paramDelimStr + strconv.Itoa(sender.id) + paramDelimStr + sender.name))
		terminate = true
	default:
		return false, err
	}

	writeLog(sender.ip, h.roomName, msgStr, 200)

	return terminate, nil
}
