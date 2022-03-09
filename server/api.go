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

	command, ok := r.URL.Query()["command"]
	if !ok || len(command) != 3 || len(command[1]) != 4 {
		w.Write([]byte("fail"))
		return
	}

	if client, found := allClients[command[0]]; found {
		client.prevMapId = command[1]
		client.prevLocations = command[2]
	} else {
		w.Write([]byte("fail"))
		return
	}

	w.Write([]byte("ok"))
}
