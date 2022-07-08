package main

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-co-op/gocron"
)

type Session struct {
	// Registered clients.
	clients map[*SessionClient]bool

	// Inbound messages from the clients.
	processMsgCh chan *SessionMessage

	// Connection requests from the clients.
	connect chan *ConnInfo

	// Unregister requests from clients.
	unregister chan *SessionClient
}

var (
	session        = newSessionWs()
	sessionClients = make(map[string]*SessionClient)
)

func initSession() {
	go session.run()

	s := gocron.NewScheduler(time.UTC)

	s.Every(5).Seconds().Do(func() {
		session.broadcast([]byte("pc" + delim + strconv.Itoa(len(sessionClients))))
		sendPartyUpdate()
	})

	s.StartAsync()
}

func newSessionWs() *Session {
	return &Session{
		clients:      make(map[*SessionClient]bool),
		processMsgCh: make(chan *SessionMessage),
		connect:      make(chan *ConnInfo),
		unregister:   make(chan *SessionClient),
	}
}

func (s *Session) serve(w http.ResponseWriter, r *http.Request) {
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

	s.connect <- &ConnInfo{Connect: conn, Ip: getIp(r), Token: playerToken}
}

func (s *Session) run() {
	http.HandleFunc("/session", s.serve)
	for {
		select {
		case conn := <-s.connect:
			var uuid string
			var name string
			var rank int
			var badge string
			var banned bool
			var muted bool
			var account bool

			if conn.Token != "" {
				uuid, name, rank, badge, banned, muted = readPlayerDataFromToken(conn.Token)
				if uuid != "" { //if we got a uuid back then we're logged in
					account = true
				}
			}
		
			if !account {
				uuid, rank, banned, muted = readPlayerData(conn.Ip)
			}
		
			if badge == "" {
				badge = "null"
			}

			if banned {
				writeErrLog(conn.Ip, "session", "player is banned")
				continue
			}

			var same_ip int
			for otherClient := range s.clients {
				if otherClient.ip == conn.Ip {
					same_ip++
				}
			}
			if same_ip >= 3 {
				writeErrLog(conn.Ip, "session", "too many connections")
				continue //don't bother with handling their connection
			}

			systemName, spriteName, spriteIndex := readPlayerGameData(uuid)

			client := &SessionClient{
				conn:        conn.Connect,
				send:        make(chan []byte, 256),
				ip:          conn.Ip,
				account:     account,
				name:        name,
				uuid:        uuid,
				rank:        rank,
				badge:       badge,
				muted:       muted,
				spriteName:  spriteName,
				spriteIndex: spriteIndex,
				systemName:  systemName,
			}
			go client.writePump()
			go client.readPump()

			client.send <- []byte("s" + delim + uuid + delim + strconv.Itoa(rank) + delim + btoa(account) + delim + badge) //"your id is %id%" message
			//register client in the structures
			s.clients[client] = true
			sessionClients[uuid] = client

			writeLog(conn.Ip, "session", "connect", 200)
		case client := <-s.unregister:
			if _, ok := s.clients[client]; ok {
				s.deleteClient(client)
				writeLog(client.ip, "session", "disconnect", 200)
				continue
			}

			writeErrLog(client.ip, "session", "attempted to unregister nil client")
		case message := <-s.processMsgCh:
			errs := s.processMsgs(message)
			if len(errs) > 0 {
				for _, err := range errs {
					writeErrLog(message.sender.ip, "session", err.Error())
				}
			}
		}
	}
}

func (s *Session) broadcast(data []byte) {
	for client := range s.clients {
		select {
		case client.send <- data:
		default:
			s.deleteClient(client)
		}
	}
}

func (s *Session) deleteClient(client *SessionClient) {
	updatePlayerGameData(client) //update database
	delete(s.clients, client)
	delete(sessionClients, client.uuid)
	close(client.send)
}

func (s *Session) processMsgs(msg *SessionMessage) []error {
	var errs []error

	if len(msg.data) > 4096 {
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

	//message processing
	msgs := strings.Split(string(msg.data), mdelim)

	for _, msgStr := range msgs {
		err := s.processMsg(msgStr, msg.sender)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (s *Session) processMsg(msgStr string, sender *SessionClient) error {
	err := errors.New(msgStr)
	msgFields := strings.Split(msgStr, delim)

	if len(msgFields) == 0 {
		return err
	}

	switch msgFields[0] {
	case "i": //player info
		err = s.handleI(msgFields, sender)
	case "name": //nick set
		err = s.handleName(msgFields, sender)
	case "ploc":
		err = s.handlePloc(msgFields, sender)
	case "gsay": //global say
		err = s.handleGSay(msgFields, sender)
	case "psay": //party say
		err = s.handlePSay(msgFields, sender)
	case "pt": //party update
		err = s.handlePt(msgFields, sender)
		if err != nil {
			sender.send <- ([]byte("pt" + delim + "null"))
		}
	case "ep": //event period
		err = s.handleEp(msgFields, sender)
	case "e": //event list
		err = s.handleE(msgFields, sender)
	default:
		return err
	}

	if err != nil {
		return err
	}

	writeLog(sender.ip, "session", msgStr, 200)

	return nil
}
