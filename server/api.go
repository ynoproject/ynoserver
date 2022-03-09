package server

import (
	"net/http"
	"strconv"
)

func StartApi() {
	http.HandleFunc("/api/admin", handleAdmin)

	http.HandleFunc("/api/ploc", handlePloc)

	http.HandleFunc("/api/players", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strconv.Itoa(len(allClients))))
	})
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	uuid, rank, _ := readPlayerData(r.Header.Get("x-forwarded-for"))
	if rank == 0 {
		w.Write([]byte("fail")) //not staff
		return
	}

	command, ok := r.URL.Query()["command"]
	if !ok || len(command) < 1 {
		w.Write([]byte("fail"))
		return
	}

	if command[0] == "ban" {
		player, ok := r.URL.Query()["player"]
		if !ok || len(player) < 1 {
			w.Write([]byte("fail"))
			return
		}

		if tryBanPlayer(uuid, player[0]) != nil {
			w.Write([]byte("fail"))
			return
		}
	} else {
		w.Write([]byte("fail")) //invalid command
	}

	w.Write([]byte("ok"))
}

func handlePloc(w http.ResponseWriter, r *http.Request) {
	uuid, _, _ := readPlayerData(r.Header.Get("x-forwarded-for"))

	prevMapId, ok := r.URL.Query()["prevMapId"]
	if !ok || len(prevMapId) < 1 || len(prevMapId[0]) != 4 {
		w.Write([]byte("fail"))
		return
	}

	prevLocations, ok := r.URL.Query()["prevLocations"]
	if !ok {
		w.Write([]byte("fail"))
		return
	}

	if client, found := allClients[uuid]; found {
		client.prevMapId = prevMapId[0]
		if len(prevLocations) > 0 {
			client.prevLocations = prevLocations[0]
		} else {
			client.prevLocations = ""
		}
	} else {
		w.Write([]byte("fail"))
		return
	}

	w.Write([]byte("ok"))
}
