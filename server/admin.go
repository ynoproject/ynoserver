/*
	Copyright (C) 2021-2024  The YNOproject Developers

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package server

import (
	"encoding/json"
	"net/http"
	"time"
)

func adminGetPlayers(w http.ResponseWriter, r *http.Request) {
	pd, err := getPlayerData(r)
	if err != nil {
		handleError(w, r, "failed to get player data")
		return
	}
	if pd.Rank < 1 {
		handleError(w, r, "access denied")
		return
	}

	response := make([]PlayerData, 0, clients.Len())
	for _, client := range clients.Get() {
		response = append(response, PlayerData{
			Uuid: client.uuid,
			Name: client.name,
			Rank: client.rank,
		})
	}

	json.NewEncoder(w).Encode(response)
}

func adminGetBansMutes(w http.ResponseWriter, r *http.Request) {
	pd, err := getPlayerData(r)
	if err != nil {
		handleError(w, r, "failed to get player data")
		return
	}
	if pd.Rank < 1 {
		handleError(w, r, "access denied")
		return
	}

	json.NewEncoder(w).Encode(getBannedMutedPlayers(r.URL.Path == "/admin/getbans"))
}

func adminBanMute(w http.ResponseWriter, r *http.Request) {
	pd, err := getPlayerData(r)
	if err != nil {
		handleError(w, r, "failed to get player data")
		return
	}
	if pd.Rank < 1 {
		handleError(w, r, "access denied")
		return
	}

	query := r.URL.Query()
	broadcast := query.Has("broadcast")

	targetUuid := query.Get("uuid")
	if targetUuid == "" {
		user := query.Get("user")
		if user == "" {
			handleError(w, r, "uuid or user not specified")
			return
		}

		uuid, err := getUuidFromName(user)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		if uuid == "" {
			handleError(w, r, "invalid user specified")
			return
		}

		targetUuid = uuid
	}

	var expiry *time.Time
	if expiryString := query.Get("expiry"); expiryString != "" {
		if parsedExpiry, err := time.Parse(time.RFC3339, expiryString); err == nil && parsedExpiry.Compare(time.Now()) > 0 {
			expiry = &parsedExpiry
		}
	}

	switch r.URL.Path {
	case "/admin/ban":
		err = tryBanPlayer(pd.Uuid, targetUuid, false, broadcast)
	case "/admin/dban":
		err = tryBanPlayer(pd.Uuid, targetUuid, true, broadcast)
	case "/admin/unban":
		err = tryUnbanPlayer(pd.Uuid, targetUuid)
	case "/admin/mute":
		err = tryMutePlayer(pd.Uuid, targetUuid, false, broadcast)
	case "/admin/unmute":
		err = tryUnmutePlayer(pd.Uuid, targetUuid)
	case "/admin/tempban":
		if expiry == nil {
			handleError(w, r, "tempban requires expiry")
			return
		}
		err = tryBanPlayerWithExpiry(pd.Uuid, targetUuid, *expiry, query.Get("reason"), broadcast)
	case "/admin/tempmute":
		if expiry == nil {
			handleError(w, r, "tempmute requires expiry")
			return
		}
		err = tryMutePlayerWithExpiry(pd.Uuid, targetUuid, *expiry, query.Get("reason"), broadcast)
	}
	if err != nil {
		handleInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func adminChangeUsername(w http.ResponseWriter, r *http.Request) {
	pd, err := getPlayerData(r)
	if err != nil {
		handleError(w, r, "failed to get player data")
		return
	}
	if pd.Rank < 1 {
		handleError(w, r, "access denied")
		return
	}

	user, newUser := r.URL.Query().Get("user"), r.URL.Query().Get("newUser")

	if user == "" {
		handleError(w, r, "user not specified")
		return
	}

	if newUser == "" {
		handleError(w, r, "new username not specified")
		return
	}

	userUuid, err := getUuidFromName(user)
	if err != nil {
		handleInternalError(w, r, err)
		return
	}
	if userUuid == "" {
		handleError(w, r, "invalid user specified")
		return
	}

	err = tryChangePlayerUsername(pd.Uuid, userUuid, newUser)
	if err != nil {
		handleInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func adminResetPw(w http.ResponseWriter, r *http.Request) {
	pd, err := getPlayerData(r)
	if err != nil {
		handleError(w, r, "failed to get player data")
		return
	}
	if pd.Rank < 1 {
		handleError(w, r, "access denied")
		return
	}

	user := r.URL.Query().Get("user")
	if user == "" {
		handleError(w, r, "user not specified")
		return
	}

	userUuid, err := getUuidFromName(user)
	if err != nil {
		handleInternalError(w, r, err)
		return
	}
	if userUuid == "" {
		handleError(w, r, "invalid user specified")
		return
	}

	userRank := getPlayerRank(userUuid)
	if userRank >= pd.Rank {
		handleError(w, r, "target rank too high")
		return
	}

	newPw, err := handleResetPw(userUuid)
	if err != nil {
		handleInternalError(w, r, err)
		return
	}

	w.Write([]byte(newPw))
}

func adminManageBadge(w http.ResponseWriter, r *http.Request) {
	pd, err := getPlayerData(r)
	if err != nil {
		handleError(w, r, "failed to get player data")
		return
	}
	if pd.Rank < 1 {
		handleError(w, r, "access denied")
		return
	}

	uuidParam := r.URL.Query().Get("uuid")
	if uuidParam == "" {
		userParam := r.URL.Query().Get("user")
		if userParam == "" {
			handleError(w, r, "uuid or user not specified")
			return
		}
		var err error
		uuidParam, err = getUuidFromName(userParam)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if uuidParam == "" {
			handleError(w, r, "invalid user specified")
			return
		}
	}

	idParam := r.URL.Query().Get("id")
	if idParam == "" {
		handleError(w, r, "badge ID not specified")
		return
	}

	var badgeExists bool

	for _, gameBadges := range badges {
		for badgeId := range gameBadges {
			if badgeId == idParam {
				badgeExists = true
				break
			}
		}
		if badgeExists {
			break
		}
	}

	if !badgeExists {
		handleError(w, r, "badge not found for the provided badge ID")
		return
	}

	if r.URL.Path == "/admin/grantbadge" {
		err = unlockPlayerBadge(uuidParam, idParam)
	} else {
		err = removePlayerBadge(uuidParam, idParam)
	}
	if err != nil {
		handleInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
