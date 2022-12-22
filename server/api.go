/*
	Copyright (C) 2021-2022  The YNOproject Developers

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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type PlayerInfo struct {
	Uuid          string `json:"uuid"`
	Name          string `json:"name"`
	Rank          int    `json:"rank"`
	Badge         string `json:"badge"`
	BadgeSlotRows int    `json:"badgeSlotRows"`
	BadgeSlotCols int    `json:"badgeSlotCols"`
	Medals        [5]int `json:"medals"`
}

func initApi() {
	http.HandleFunc("/admin/getplayers", adminGetOnlinePlayers)
	http.HandleFunc("/admin/getbans", adminGetBans)
	http.HandleFunc("/admin/getmutes", adminGetMutes)
	http.HandleFunc("/admin/ban", adminBan)
	http.HandleFunc("/admin/mute", adminMute)
	http.HandleFunc("/admin/unban", adminUnban)
	http.HandleFunc("/admin/unmute", adminUnmute)

	http.HandleFunc("/api/admin", handleAdmin)
	http.HandleFunc("/api/party", handleParty)
	http.HandleFunc("/api/saveSync", handleSaveSync)
	http.HandleFunc("/api/vm", handleVm)
	http.HandleFunc("/api/badge", handleBadge)

	http.HandleFunc("/api/register", handleRegister)
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/logout", handleLogout)
	http.HandleFunc("/api/changepw", handleChangePw)

	http.HandleFunc("/api/2kki", func(w http.ResponseWriter, r *http.Request) {
		if serverConfig.GameName != "2kki" {
			handleError(w, r, "endpoint not supported")
			return
		}

		actionParam := r.URL.Query().Get("action")
		if actionParam == "" {
			handleError(w, r, "action not specified")
			return
		}

		query := r.URL.Query()
		query.Del("action")

		queryString := query.Encode()

		var response string

		err := db.QueryRow("SELECT response FROM 2kkiApiQueries WHERE action = ? AND query = ? AND CURRENT_TIMESTAMP() < timestampExpired", actionParam, queryString).Scan(&response)
		if err != nil {
			if err != sql.ErrNoRows {
				handleInternalError(w, r, err)
				return
			}

			url := "https://2kki.app/" + actionParam
			if queryString != "" {
				url += "?" + queryString
			}

			resp, err := http.Get(url)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}

			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}

			if strings.HasPrefix(string(body), "{\"error\"") || strings.HasPrefix(string(body), "<!DOCTYPE html>") {
				writeErrLog(getIp(r), r.URL.Path, "received error response from Yume 2kki Explorer API: "+string(body))
			} else {
				var interval string
				// Shorter expiration for map queries returning unknown location in case of new maps that haven't yet been added to the wiki
				if actionParam == "getMapLocationNames" && response == "[]" {
					interval = "1 HOUR"
				} else {
					interval = "7 DAY"
				}
				_, err = db.Exec("INSERT INTO 2kkiApiQueries (action, query, response, timestampExpired) VALUES (?, ?, ?, DATE_ADD(CURRENT_TIMESTAMP(), INTERVAL "+interval+")) ON DUPLICATE KEY UPDATE response = ?, timestampExpired = DATE_ADD(CURRENT_TIMESTAMP(), INTERVAL "+interval+")", actionParam, queryString, string(body), string(body))
				if err != nil {
					writeErrLog(getIp(r), r.URL.Path, err.Error())
				}
			}

			w.Write(body)
			return
		}

		w.Write([]byte(response))
	})

	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		var uuid string
		var name string
		var rank int
		var badge string
		var badgeSlotRows int
		var badgeSlotCols int
		var medals [5]int

		token := r.Header.Get("Authorization")
		if token == "" {
			uuid, name, rank = getPlayerInfo(getIp(r))
		} else {
			uuid, name, rank, badge, badgeSlotRows, badgeSlotCols = getPlayerInfoFromToken(token)
			medals = getPlayerMedals(uuid)
		}

		// guest accounts with no playerGameData records will return nothing
		// if uuid is empty it breaks fetchAndUpdatePlayerInfo in forest-orb
		if uuid == "" {
			uuid = "null"
		}

		playerInfo := PlayerInfo{
			Uuid:          uuid,
			Name:          name,
			Rank:          rank,
			Badge:         badge,
			BadgeSlotRows: badgeSlotRows,
			BadgeSlotCols: badgeSlotCols,
			Medals:        medals,
		}
		playerInfoJson, err := json.Marshal(playerInfo)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write(playerInfoJson)
	})

	http.HandleFunc("/api/players", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strconv.Itoa(getPlayerCount())))
	})
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var rank int

	uuid, _, rank, _, _, _ = getPlayerDataFromToken(r.Header.Get("Authorization"))
	if rank == 0 {
		handleError(w, r, "access denied")
		return
	}

	commandParam := r.URL.Query().Get("command")
	if commandParam == "" {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam {
	case "grantbadge", "revokebadge":
		playerParam := r.URL.Query().Get("player")
		if playerParam == "" {
			handleError(w, r, "player not specified")
			return
		}

		idParam := r.URL.Query().Get("id")
		if playerParam == "" {
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

		var err error
		if commandParam == "grantbadge" {
			err = unlockPlayerBadge(playerParam, idParam)
		} else {
			err = removePlayerBadge(playerParam, idParam)
		}
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "resetpw":
		if getPlayerRank(uuid) < 2 {
			handleError(w, r, "access denied")
			return
		}

		playerParam := r.URL.Query().Get("player")
		if playerParam == "" {
			handleError(w, r, "player not specified")
			return
		}

		newPw, err := handleResetPw(playerParam)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		w.Write([]byte(newPw))
	default:
		handleError(w, r, "unknown command")
		return
	}

	w.Write([]byte("ok"))
}

func handleParty(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var rank int
	var banned bool

	token := r.Header.Get("Authorization")
	if token == "" {
		uuid, banned, _ = getOrCreatePlayerData(getIp(r))
	} else {
		uuid, _, rank, _, banned, _ = getPlayerDataFromToken(token)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	commandParam := r.URL.Query().Get("command")
	if commandParam == "" {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam {
	case "id":
		partyId, err := getPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(strconv.Itoa(partyId)))
		return
	case "list":
		partyListData, err := getAllPartyData(true)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		partyListDataJson, err := json.Marshal(partyListData)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write(partyListDataJson)
		return
	case "description":
		partyIdParam := r.URL.Query().Get("partyId")
		if partyIdParam == "" {
			handleError(w, r, "partyId not specified")
			return
		}
		partyId, err := strconv.Atoi(partyIdParam)
		if err != nil {
			handleError(w, r, "invalid partyId value")
			return
		}
		description, err := getPartyDescription(partyId)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(description))
		return
	case "create", "update":
		partyId, err := getPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		create := commandParam == "create"
		if create {
			if partyId > 0 {
				handleError(w, r, "player already in a party")
				return
			}
		} else {
			if partyId == 0 {
				handleError(w, r, "player not in a party")
				return
			}
			ownerUuid, err := getPartyOwnerUuid(partyId)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			if ownerUuid != uuid {
				handleError(w, r, "attempted party update from non-owner")
				return
			}
		}
		nameParam := r.URL.Query().Get("name")
		if nameParam == "" {
			handleError(w, r, "name not specified")
			return
		}
		if len(nameParam) > 255 {
			handleError(w, r, "name too long")
			return
		}
		var description string
		descriptionParam := r.URL.Query().Get("description")
		if descriptionParam != "" {
			description = descriptionParam
		}
		var public bool
		publicParam := r.URL.Query().Get("public")
		if publicParam != "" {
			public = true
		}
		var pass string
		if !public {
			passParam := r.URL.Query().Get("pass")
			if passParam != "" {
				if len(passParam) > 255 {
					handleError(w, r, "pass too long")
					return
				}
				pass = passParam
			}
		}
		themeParam := r.URL.Query().Get("theme")
		if themeParam == "" {
			handleError(w, r, "theme not specified")
			return
		}
		if !gameAssets.IsValidSystem(themeParam, true) {
			handleError(w, r, "invalid system name for theme")
			return
		}
		if create {
			partyId, err = createPartyData(nameParam, public, pass, themeParam, description, uuid)
		} else {
			err = updatePartyData(partyId, nameParam, public, pass, themeParam, description, uuid)
		}
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if create {
			err = createPlayerParty(partyId, uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			w.Write([]byte(strconv.Itoa(partyId)))
			return
		}
	case "join":
		partyIdParam := r.URL.Query().Get("partyId")
		if partyIdParam == "" {
			handleError(w, r, "partyId not specified")
			return
		}
		partyId, err := strconv.Atoi(partyIdParam)
		if err != nil {
			handleError(w, r, "invalid partyId value")
			return
		}
		if rank == 0 {
			public, err := getPartyPublic(partyId)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			if !public {
				passParam := r.URL.Query().Get("pass")
				if passParam == "" {
					handleError(w, r, "pass not specified")
					return
				}
				partyPass, err := getPartyPass(partyId)
				if err != nil {
					handleInternalError(w, r, err)
				}
				if partyPass != "" && passParam != partyPass {
					http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}
		playerPartyId, err := getPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if playerPartyId > 0 {
			handleError(w, r, "player already in a party")
			return
		}
		err = createPlayerParty(partyId, uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "leave":
		partyId, err := getPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if partyId == 0 {
			handleError(w, r, "player not in a party")
			return
		}
		err = handlePartyMemberLeave(partyId, uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "kick", "transfer":
		kick := commandParam == "kick"
		partyId, err := getPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if partyId == 0 {
			handleError(w, r, "player not in a party")
			return
		}
		ownerUuid, err := getPartyOwnerUuid(partyId)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if ownerUuid != uuid {
			if kick {
				handleError(w, r, "attempted party kick non-owner")
			} else {
				handleError(w, r, "attempted owner transfer from non-owner")
			}
			return
		}
		playerParam := r.URL.Query().Get("player")
		if playerParam == "" {
			handleError(w, r, "player not specified")
			return
		}
		playerPartyId, err := getPlayerPartyId(playerParam)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if playerPartyId != partyId {
			if kick {
				handleError(w, r, "specified player to kick not in same party")
			} else {
				handleError(w, r, "specified player to transfer owner not in same party")
			}
			return
		}
		if kick {
			err = clearPlayerParty(playerParam)
		} else {
			err = setPartyOwner(partyId, playerParam)
		}
		if err != nil {
			handleInternalError(w, r, nil)
		}
	case "disband":
		partyId, err := getPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		ownerUuid, err := getPartyOwnerUuid(partyId)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if ownerUuid != uuid {
			handleError(w, r, "attempted party disband from non-owner")
			return
		}
		err = deletePartyAndMembers(partyId)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	default:
		handleError(w, r, "unknown command")
		return
	}

	w.Write([]byte("ok"))
}

func handlePartyMemberLeave(partyId int, playerUuid string) error {
	ownerUuid, err := getPartyOwnerUuid(partyId)
	if err != nil {
		return err
	}

	err = clearPlayerParty(playerUuid)
	if err != nil {
		return err
	}

	deleted, err := checkDeleteOrphanedParty(partyId)
	if err != nil {
		return err
	}
	if !deleted && playerUuid == ownerUuid {
		err = assumeNextPartyOwner(partyId)
		if err != nil {
			return err
		}
	}

	return nil
}

func handleSaveSync(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var banned bool

	token := r.Header.Get("Authorization")
	if token == "" {
		handleError(w, r, "token not specified")
		return
	} else {
		uuid, _, _, _, banned, _ = getPlayerDataFromToken(token)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	commandParam := r.URL.Query().Get("command")
	if commandParam == "" {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam {
	case "timestamp":
		timestamp, err := getSaveDataTimestamp(uuid)
		if err != nil {
			if err == sql.ErrNoRows {
				return
			}
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(timestamp.Format(time.RFC3339)))
		return
	case "get":
		saveData, err := getSaveData(uuid)
		if err != nil {
			if err == sql.ErrNoRows {
				w.Write([]byte("{}"))
				return
			}
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(saveData))
		return
	case "push":
		timestampParam := r.URL.Query().Get("timestamp")
		if timestampParam == "" {
			handleError(w, r, "timestamp not specified")
			return
		}
		timestamp, err := time.Parse(time.RFC3339, timestampParam)
		if err != nil {
			handleError(w, r, "invalid timestamp value")
			return
		}
		data, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil || len(data) > 1024*1024*8 {
			handleError(w, r, "invalid data")
			return
		}
		err = createGameSaveData(uuid, timestamp, string(data))
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		return
	case "clear":
		err := clearGameSaveData(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	default:
		handleError(w, r, "unknown command")
		return
	}

	w.Write([]byte("ok"))
}

func handleVm(w http.ResponseWriter, r *http.Request) {
	idParam := r.URL.Query().Get("id")
	if idParam == "" {
		handleError(w, r, "id not specified")
		return
	}

	eventVmId, err := strconv.Atoi(idParam)
	if err != nil {
		handleInternalError(w, r, err)
		return
	}

	mapId, eventId, err := getEventVmInfo(eventVmId)
	if err != nil {
		handleInternalError(w, r, err)
		return
	}

	fileBytes, err := os.ReadFile("vms/Map" + fmt.Sprintf("%04d", mapId) + "_EV" + fmt.Sprintf("%04d", eventId) + ".png")
	if err != nil {
		handleInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(fileBytes)
}

func handleBadge(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var name string
	var rank int
	var badge string
	var badgeSlotRows int
	var badgeSlotCols int
	var banned bool

	commandParam := r.URL.Query().Get("command")
	if commandParam == "" {
		handleError(w, r, "command not specified")
		return
	}
	token := r.Header.Get("Authorization")
	if token == "" {
		if commandParam == "list" || commandParam == "playerSlotList" {
			uuid, banned, _ = getOrCreatePlayerData(getIp(r))
		} else {
			handleError(w, r, "token not specified")
			return
		}
	} else {
		uuid, name, rank, badge, banned, _ = getPlayerDataFromToken(token)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	if strings.HasPrefix(commandParam, "slot") {
		badgeSlotRows, badgeSlotCols = getPlayerBadgeSlotCounts(name)
	}

	switch commandParam {
	case "set", "slotSet":
		idParam := r.URL.Query().Get("id")
		if idParam == "" {
			handleError(w, r, "id not specified")
			return
		}

		if idParam != badge {
			var unlocked bool

			switch idParam {
			case "null":
				unlocked = true
			default:
				tags, err := getPlayerTags(uuid)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				badgeData, err := getPlayerBadgeData(uuid, rank, tags, true, true)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				var badgeFound bool
				for _, badge := range badgeData {
					if badge.BadgeId == idParam {
						badgeFound = true
						unlocked = badge.Unlocked
						break
					}
				}
				if !badgeFound {
					handleError(w, r, "unknown badge")
					return
				}
			}

			if rank < 2 && !unlocked {
				handleError(w, r, "specified badge is locked")
				return
			}
		}

		if commandParam == "set" {
			err := setPlayerBadge(uuid, idParam)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		} else {
			rowParam := r.URL.Query().Get("row")
			if rowParam == "" {
				handleError(w, r, "row not specified")
				return
			}

			colParam := r.URL.Query().Get("col")
			if colParam == "" {
				handleError(w, r, "col not specified")
				return
			}

			slotRow, err := strconv.Atoi(rowParam)
			if err != nil || slotRow == 0 || slotRow > badgeSlotRows {
				handleError(w, r, "invalid row value")
				return
			}

			slotCol, err := strconv.Atoi(colParam)
			if err != nil || slotCol == 0 || slotCol > badgeSlotCols {
				handleError(w, r, "invalid col value")
				return
			}

			err = setPlayerBadgeSlot(uuid, idParam, slotRow, slotCol)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
	case "list":
		var tags []string
		if token != "" {
			var err error
			tags, err = getPlayerTags(uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
		if r.URL.Query().Get("simple") == "true" {
			simpleBadgeData, err := getSimplePlayerBadgeData(uuid, rank, tags, token != "")
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			simpleBadgeDataJson, err := json.Marshal(simpleBadgeData)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			w.Write(simpleBadgeDataJson)
		} else {
			if token == "" {
				handleError(w, r, "cannot retrieve player badge data for guest player")
				return
			}
			badgeData, err := getPlayerBadgeData(uuid, rank, tags, true, false)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			badgeDataJson, err := json.Marshal(badgeData)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			w.Write(badgeDataJson)
		}
		return
	case "new":
		var tags []string
		if token != "" {
			var err error
			tags, err = getPlayerTags(uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
		newUnlockedBadgeIds, err := getPlayerNewUnlockedBadgeIds(uuid, rank, tags)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if len(newUnlockedBadgeIds) != 0 {
			err := updatePlayerBadgeSlotCounts(uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
		newUnlockedBadgeIdsJson, err := json.Marshal(newUnlockedBadgeIds)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write(newUnlockedBadgeIdsJson)
		return
	case "slotList":
		badgeSlots, err := getPlayerBadgeSlots(name, badgeSlotRows, badgeSlotCols)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		badgeSlotsJson, err := json.Marshal(badgeSlots)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write(badgeSlotsJson)
		return
	case "playerSlotList":
		playerParam := r.URL.Query().Get("player")
		if playerParam == "" {
			handleError(w, r, "player not specified")
			return
		}

		playerBadgeSlotRows, playerBadgeSlotCols := getPlayerBadgeSlotCounts(playerParam)

		badgeSlots, err := getPlayerBadgeSlots(playerParam, playerBadgeSlotRows, playerBadgeSlotCols)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		badgeSlotsJson, err := json.Marshal(badgeSlots)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write(badgeSlotsJson)
		return
	default:
		handleError(w, r, "unknown command")
		return
	}

	w.Write([]byte("ok"))
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	// GET params user, password
	user, password := r.URL.Query().Get("user"), r.URL.Query().Get("password")
	if user == "" || len(user) > 12 || !isOkString(user) || password == "" || len(password) > 72 {
		handleError(w, r, "bad response")
		return
	}

	ip := getIp(r)

	if isVpn(ip) {
		handleError(w, r, "vpn not permitted")
	}

	if isIpBanned(ip) {
		handleError(w, r, "banned users cannot create accounts")
		return
	}

	var userExists int
	db.QueryRow("SELECT COUNT(*) FROM accounts WHERE user = ?", user).Scan(&userExists)

	if userExists > 0 {
		handleError(w, r, "user exists")
		return
	}

	var uuid string
	db.QueryRow("SELECT uuid FROM players WHERE ip = ?", ip).Scan(&uuid) // no row causes a non-fatal error, uuid is still unset so it doesn't matter
	if uuid == "" {
		uuid, _, _ = getOrCreatePlayerData(ip)
	}

	db.Exec("UPDATE players SET ip = NULL WHERE ip = ?", ip) // set ip to null to disable ip-based login

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		handleError(w, r, "bcrypt error")
		return
	}

	db.Exec("INSERT INTO accounts (ip, timestampRegistered, uuid, user, pass) VALUES (?, ?, ?, ?, ?)", ip, time.Now(), uuid, user, hashedPassword)

	w.Write([]byte("ok"))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	// GET params user, password
	user, password := r.URL.Query().Get("user"), r.URL.Query().Get("password")
	if user == "" || !isOkString(user) || password == "" || len(password) > 72 {
		handleError(w, r, "bad response")
		return
	}

	var userPassHash string
	db.QueryRow("SELECT pass FROM accounts WHERE user = ?", user).Scan(&userPassHash)

	if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password)) != nil {
		handleError(w, r, "bad login")
		return
	}

	token := randString(32)
	db.Exec("INSERT INTO playerSessions (sessionId, uuid, expiration) (SELECT ?, uuid, DATE_ADD(NOW(), INTERVAL 30 DAY) FROM accounts WHERE user = ?)", token, user)
	db.Exec("UPDATE accounts SET timestampLoggedIn = CURRENT_TIMESTAMP() WHERE user = ?", user)

	w.Write([]byte(token))
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")

	if token == "" {
		handleError(w, r, "token not specified")
		return
	}

	if getUuidFromToken(token) == "" {
		handleError(w, r, "invalid token")
		return
	}

	db.Exec("DELETE FROM playerSessions WHERE sessionId = ?", token)

	w.Write([]byte("ok"))
}

func handleChangePw(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")

	if token == "" {
		handleError(w, r, "token not specified")
		return
	}

	_, loginUser, rank, _, _, _ := getPlayerInfoFromToken(token)

	// GET params user, new password
	user, newPassword := r.URL.Query().Get("user"), r.URL.Query().Get("newPassword")

	var username string
	if rank < 1 || user == "" {
		username = loginUser

		// GET param password
		password := r.URL.Query().Get("password")

		if username == "" || !isOkString(username) || password == "" || len(password) > 72 || newPassword == "" || len(newPassword) > 72 {
			handleError(w, r, "bad response")
			return
		}

		var userPassHash string
		db.QueryRow("SELECT pass FROM accounts WHERE user = ?", username).Scan(&userPassHash)

		if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password)) != nil {
			handleError(w, r, "bad login")
			return
		}
	} else {
		if !isOkString(user) || newPassword == "" || len(newPassword) > 72 {
			handleError(w, r, "bad response")
			return
		}

		username = user
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		handleError(w, r, "bcrypt error")
		return
	}

	db.Exec("UPDATE accounts SET pass = ? WHERE user = ?", hashedPassword, username)

	w.Write([]byte("ok"))
}

func handleResetPw(user string) (newPassword string, err error) {
	var userCount int
	db.QueryRow("SELECT COUNT(*) FROM accounts WHERE user = ?", user).Scan(&userCount)

	if userCount == 0 {
		return "", errors.New("user not found")
	}

	newPassword = randString(8)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.New("bcrypt error")
	}

	db.Exec("UPDATE accounts SET pass = ? WHERE user = ?", hashedPassword, user)

	return newPassword, nil
}

func handleError(w http.ResponseWriter, r *http.Request, payload string) {
	writeErrLog(getIp(r), r.URL.Path, payload)
	http.Error(w, payload, http.StatusBadRequest)
}

func handleInternalError(w http.ResponseWriter, r *http.Request, err error) {
	writeErrLog(getIp(r), r.URL.Path, err.Error())
	http.Error(w, "400 - Bad Request", http.StatusBadRequest)
}
