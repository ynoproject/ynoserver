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
		handleError(w, r)
		return
	}

	command, ok := r.URL.Query()["command"]
	if !ok || len(command) < 1 {
		handleError(w, r)
		return
	}

	if command[0] == "ban" {
		player, ok := r.URL.Query()["player"]
		if !ok || len(player) < 1 {
			handleError(w, r)
			return
		}

		if tryBanPlayer(uuid, player[0]) != nil {
			handleError(w, r)
			return
		}
	} else {
		handleError(w, r) //invalid command
	}

	w.Write([]byte("ok"))
}

func handlePloc(w http.ResponseWriter, r *http.Request) {
	uuid, _, _ := readPlayerData(r.Header.Get("x-forwarded-for"))

	prevMapId, ok := r.URL.Query()["prevMapId"]
	if !ok || len(prevMapId) < 1 || len(prevMapId[0]) != 4 {
		handleError(w, r)
		return
	}

	prevLocations, ok := r.URL.Query()["prevLocations"]
	if !ok {
		handleError(w, r)
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
		handleError(w, r)
		return
	}

	w.Write([]byte("ok"))
}

func handleError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("400 - Bad Request"))
}
