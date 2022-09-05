package main

import (
	"encoding/json"
	"net/http"
)

type AdminPlayersResponse struct {
	Players []PlayerInfo `json:"player"`
}

func tokenIsRank(token string, rank int) bool {
	_, _, playerRank, _, _, _ := readPlayerDataFromToken(token)

	return playerRank >= rank
}

func adminGetOnlinePlayers(w http.ResponseWriter, r *http.Request) {
	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

	var response AdminPlayersResponse

	for _, client := range sessionClients {
		player := PlayerInfo{
			Uuid: client.uuid,
			Name: client.name,
			Rank: client.rank,
		}

		response.Players = append(response.Players, player)
	}
	
	responseJson, err := json.Marshal(response)
	if err != nil {
		handleError(w, r, "error while marshaling")
	}

	w.Write(responseJson)
}

func adminGetBans(w http.ResponseWriter, r *http.Request) {
	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

}

func adminGetMutes(w http.ResponseWriter, r *http.Request) {
	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

}

func adminBan(w http.ResponseWriter, r *http.Request) {
	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

}

func adminMute(w http.ResponseWriter, r *http.Request) {
	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

}

func adminUnban(w http.ResponseWriter, r *http.Request) {
	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

}

func adminUnmute(w http.ResponseWriter, r *http.Request) {
	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

}