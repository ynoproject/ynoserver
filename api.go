package main

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

	"github.com/thanhpk/randstr"
	"golang.org/x/crypto/bcrypt"
)

type PlayerInfo struct {
	Uuid          string `json:"uuid"`
	Name          string `json:"name"`
	Rank          int    `json:"rank"`
	Badge         string `json:"badge"`
	BadgeSlotRows int    `json:"badgeSlotRows"`
	BadgeSlotCols int    `json:"badgeSlotCols"`
}

type SyncedPicsInfo struct {
	PictureNames    []string `json:"pictureNames"`
	PicturePrefixes []string `json:"picturePrefixes"`
}

var (
	syncedPicsResponse []byte //cached response
)

func initApi() {
	http.HandleFunc("/admin/getplayers", adminGetOnlinePlayers)
	http.HandleFunc("/admin/getbans", adminGetBans)
	http.HandleFunc("/admin/getbans", adminGetBans)
	http.HandleFunc("/admin/getmutes", adminGetMutes)
	http.HandleFunc("/admin/ban", adminBan)
	http.HandleFunc("/admin/mute", adminMute)
	http.HandleFunc("/admin/unban", adminUnban)
	http.HandleFunc("/admin/unmute", adminUnmute)

	http.HandleFunc("/api/admin", handleAdmin)
	http.HandleFunc("/api/party", handleParty)
	http.HandleFunc("/api/saveSync", handleSaveSync)
	http.HandleFunc("/api/events", handleEvents)
	http.HandleFunc("/api/badge", handleBadge)
	http.HandleFunc("/api/ranking", handleRanking)
	http.HandleFunc("/api/syncedPics", handleSyncedPics)

	http.HandleFunc("/api/register", handleRegister)
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/logout", handleLogout)
	http.HandleFunc("/api/changepw", handleChangePw)

	http.HandleFunc("/api/2kki", func(w http.ResponseWriter, r *http.Request) {
		if config.gameName != "2kki" {
			handleError(w, r, "endpoint not supported")
			return
		}

		actionParam, ok := r.URL.Query()["action"]
		if !ok || len(actionParam) < 1 {
			handleError(w, r, "action not specified")
			return
		}

		query := r.URL.Query()
		query.Del("action")

		queryString := query.Encode()

		var response string

		err := db.QueryRow("SELECT response FROM 2kkiApiQueries WHERE action = ? AND query = ? AND CURRENT_TIMESTAMP() < timestampExpired", actionParam[0], queryString).Scan(&response)
		if err != nil {
			if err == sql.ErrNoRows {
				url := "https://2kki.app/" + actionParam[0]
				if len(queryString) > 0 {
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
					_, err = db.Exec("INSERT INTO 2kkiApiQueries (action, query, response, timestampExpired) VALUES (?, ?, ?, DATE_ADD(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)) ON DUPLICATE KEY UPDATE response = ?, timestampExpired = DATE_ADD(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)", actionParam[0], queryString, string(body), string(body))
					if err != nil {
						writeErrLog(getIp(r), r.URL.Path, err.Error())
					}
				}

				w.Write(body)
				return
			} else {
				handleInternalError(w, r, err)
				return
			}
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

		token := r.Header.Get("Authorization")
		if token == "" {
			uuid, name, rank = readPlayerInfo(getIp(r))
		} else {
			uuid, name, rank, badge, badgeSlotRows, badgeSlotCols = readPlayerInfoFromToken(token)
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
		}
		playerInfoJson, err := json.Marshal(playerInfo)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(playerInfoJson))
	})
	http.HandleFunc("/api/players", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strconv.Itoa(len(sessionClients))))
	})
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	var uuid string

	if tokenIsRank(r.Header.Get("Authorization"), 1) {
		handleError(w, r, "access denied")
		return
	}

	commandParam, ok := r.URL.Query()["command"]
	if !ok || len(commandParam) < 1 {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam[0] {
	case "ban":
		playerParam, ok := r.URL.Query()["player"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "player not specified")
			return
		}

		err := tryBanPlayer(uuid, playerParam[0])
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "mute":
		// deprecated
		playerParam, ok := r.URL.Query()["player"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "player not specified")
			return
		}

		err := tryMutePlayer(uuid, playerParam[0])
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "unmute":
		// deprecated
		playerParam, ok := r.URL.Query()["player"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "player not specified")
			return
		}

		err := tryUnmutePlayer(uuid, playerParam[0])
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "grantbadge":
		fallthrough
	case "revokebadge":
		playerParam, ok := r.URL.Query()["player"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "player not specified")
			return
		}

		idParam, ok := r.URL.Query()["id"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "badge ID not specified")
			return
		}

		var badgeExists bool

		for _, gameBadges := range badges {
			for badgeId := range gameBadges {
				if badgeId == idParam[0] {
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
		if commandParam[0] == "grantbadge" {
			err = unlockPlayerBadge(playerParam[0], idParam[0])
		} else {
			err = removePlayerBadge(playerParam[0], idParam[0])
		}
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "resetpw":
		if readPlayerRank(uuid) < 2 {
			handleError(w, r, "access denied")
			return
		}

		playerParam, ok := r.URL.Query()["player"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "player not specified")
			return
		}

		newPw, err := handleResetPw(playerParam[0])
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		w.Write([]byte(newPw))
	case "restart":
		if readPlayerRank(uuid) < 2 {
			handleError(w, r, "access denied")
			return
		}

		w.Write([]byte("ok")) //can't send it later
		writeLog(getIp(r), r.URL.Path, "restarting server", 200)

		os.Exit(0)
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
		uuid, rank, banned, _ = readPlayerData(getIp(r))
	} else {
		uuid, _, rank, _, banned, _ = readPlayerDataFromToken(token)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	commandParam, ok := r.URL.Query()["command"]
	if !ok || len(commandParam) < 1 {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam[0] {
	case "id":
		partyId, err := readPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(strconv.Itoa(partyId)))
		return
	case "list":
		partyListData, err := readAllPartyData(true)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		partyListDataJson, err := json.Marshal(partyListData)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(partyListDataJson))
		return
	case "description":
		partyIdParam, ok := r.URL.Query()["partyId"]
		if !ok || len(partyIdParam) < 1 {
			handleError(w, r, "partyId not specified")
			return
		}
		partyId, err := strconv.Atoi(partyIdParam[0])
		if err != nil {
			handleError(w, r, "invalid partyId value")
			return
		}
		description, err := readPartyDescription(partyId)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(description))
		return
	case "create":
		fallthrough
	case "update":
		partyId, err := readPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		create := commandParam[0] == "create"
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
			ownerUuid, err := readPartyOwnerUuid(partyId)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			if ownerUuid != uuid {
				handleError(w, r, "attempted party update from non-owner")
				return
			}
		}
		nameParam, ok := r.URL.Query()["name"]
		if !ok || len(nameParam) < 1 {
			handleError(w, r, "name not specified")
			return
		}
		if len(nameParam[0]) > 255 {
			handleError(w, r, "name too long")
			return
		}
		var description string
		descriptionParam, ok := r.URL.Query()["description"]
		if ok && len(descriptionParam) >= 1 {
			description = descriptionParam[0]
		}
		var public bool
		publicParam, ok := r.URL.Query()["public"]
		if ok && len(publicParam) >= 1 {
			public = true
		}
		var pass string
		if !public {
			passParam, ok := r.URL.Query()["pass"]
			if ok && len(passParam) >= 1 {
				if len(passParam[0]) > 255 {
					handleError(w, r, "pass too long")
					return
				}
				pass = passParam[0]
			}
		}
		themeParam, ok := r.URL.Query()["theme"]
		if !ok || len(themeParam) < 1 {
			handleError(w, r, "theme not specified")
			return
		}
		if !isValidSystem(themeParam[0], true) {
			handleError(w, r, "invalid system name for theme")
			return
		}
		if create {
			partyId, err = createPartyData(nameParam[0], public, pass, themeParam[0], description, uuid)
		} else {
			err = updatePartyData(partyId, nameParam[0], public, pass, themeParam[0], description, uuid)
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
		partyIdParam, ok := r.URL.Query()["partyId"]
		if !ok || len(partyIdParam) < 1 {
			handleError(w, r, "partyId not specified")
			return
		}
		partyId, err := strconv.Atoi(partyIdParam[0])
		if err != nil {
			handleError(w, r, "invalid partyId value")
			return
		}
		if rank == 0 {
			public, err := readPartyPublic(partyId)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			if !public {
				passParam, ok := r.URL.Query()["pass"]
				if !ok || len(passParam) < 1 {
					handleError(w, r, "pass not specified")
					return
				}
				partyPass, err := readPartyPass(partyId)
				if err != nil {
					handleInternalError(w, r, err)
				}
				if partyPass != "" && passParam[0] != partyPass {
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("401 - Unauthorized"))
					return
				}
			}
		}
		playerPartyId, err := readPlayerPartyId(uuid)
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
		partyId, err := readPlayerPartyId(uuid)
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
	case "kick":
		fallthrough
	case "transfer":
		kick := commandParam[0] == "kick"
		partyId, err := readPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if partyId == 0 {
			handleError(w, r, "player not in a party")
			return
		}
		ownerUuid, err := readPartyOwnerUuid(partyId)
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
		playerParam, ok := r.URL.Query()["player"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "player not specified")
			return
		}
		playerUuid := playerParam[0]
		playerPartyId, err := readPlayerPartyId(playerUuid)
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
			err = clearPlayerParty(playerUuid)
		} else {
			err = setPartyOwner(partyId, playerUuid)
		}
		if err != nil {
			handleInternalError(w, r, nil)
		}
	case "disband":
		partyId, err := readPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		ownerUuid, err := readPartyOwnerUuid(partyId)
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
	ownerUuid, err := readPartyOwnerUuid(partyId)
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
		uuid, _, _, _, banned, _ = readPlayerDataFromToken(token)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	commandParam, ok := r.URL.Query()["command"]
	if !ok || len(commandParam) < 1 {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam[0] {
	case "timestamp":
		timestamp, err := readSaveDataTimestamp(uuid)
		if err != nil {
			if err == sql.ErrNoRows {
				w.Write([]byte(""))
				return
			}
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(timestamp.Format(time.RFC3339)))
		return
	case "get":
		saveData, err := readSaveData(uuid)
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
		timestampParam, ok := r.URL.Query()["timestamp"]
		if !ok || len(timestampParam) < 1 {
			handleError(w, r, "timestamp not specified")
			return
		}
		timestamp, err := time.Parse(time.RFC3339, timestampParam[0])
		if err != nil {
			handleError(w, r, "invalid timestamp value")
			return
		}
		data, err := io.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil || len(data) > 1024 * 1024 * 2 {
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

func handleEvents(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var banned bool

	token := r.Header.Get("Authorization")
	if token == "" {
		handleError(w, r, "token not specified")
		return
	} else {
		uuid, _, _, _, banned, _ = readPlayerDataFromToken(token)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	commandParam, ok := r.URL.Query()["command"]
	if !ok || len(commandParam) < 1 {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam[0] {
	case "exp":
		periodId, err := readCurrentEventPeriodId()
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		playerEventExpData, err := readPlayerEventExpData(periodId, uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		playerEventExpDataJson, err := json.Marshal(playerEventExpData)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(playerEventExpDataJson))
	case "claim":
		locationParam, ok := r.URL.Query()["location"]
		if !ok || len(locationParam) < 1 {
			handleError(w, r, "location not specified")
			return
		}
		var free bool
		freeParam, ok := r.URL.Query()["free"]
		if ok && len(freeParam) >= 1 && freeParam[0] != "0" {
			free = true
		}
		periodId, err := readCurrentEventPeriodId()
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		ret := -1
		if _, found := hubClients[uuid]; found {
			if !free {
				exp, err := tryCompleteEventLocation(periodId, uuid, locationParam[0])
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				if exp < 0 {
					handleError(w, r, "unexpected state")
					return
				}
				ret = exp
			} else {
				complete, err := tryCompletePlayerEventLocation(periodId, uuid, locationParam[0])
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				if complete {
					ret = 0
				}
			}
			currentEventLocationsData, err := readCurrentPlayerEventLocationsData(periodId, uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			var hasIncompleteEvent bool
			for _, currentEventLocation := range currentEventLocationsData {
				if !currentEventLocation.Complete {
					hasIncompleteEvent = true
					break
				}
			}
			if !hasIncompleteEvent && config.gameName == "2kki" {
				addPlayer2kkiEventLocation(-1, 2, 0, 0, uuid)
			}
		} else {
			handleError(w, r, "unexpected state")
			return
		}
		w.Write([]byte(strconv.Itoa(ret)))
	case "vm":
		idParam, ok := r.URL.Query()["id"]
		if !ok || len(idParam) < 1 {
			handleError(w, r, "id not specified")
			return
		}

		eventVmId, err := strconv.Atoi(idParam[0])
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		mapId, eventId, err := readEventVmInfo(eventVmId)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		fileBytes, err := os.ReadFile("vms/Map" + fmt.Sprintf("%04d", mapId) + "_EV" + fmt.Sprintf("%04d", eventId) + ".png")
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(fileBytes)
	default:
		handleError(w, r, "unknown command")
	}
}

func handleBadge(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var name string
	var rank int
	var badge string
	var badgeSlotRows int
	var badgeSlotCols int
	var banned bool

	commandParam, ok := r.URL.Query()["command"]
	if !ok || len(commandParam) < 1 {
		handleError(w, r, "command not specified")
		return
	}
	token := r.Header.Get("Authorization")
	if token == "" {
		if commandParam[0] == "list" || commandParam[0] == "playerSlotList" {
			uuid, rank, banned, _ = readPlayerData(getIp(r))
		} else {
			handleError(w, r, "token not specified")
			return
		}
	} else {
		uuid, name, rank, badge, banned, _ = readPlayerDataFromToken(token)
	}

	if strings.HasPrefix(commandParam[0], "slot") {
		badgeSlotRows, badgeSlotCols = readPlayerBadgeSlotCounts(name)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	switch commandParam[0] {
	case "set":
		fallthrough
	case "slotSet":
		idParam, ok := r.URL.Query()["id"]
		if !ok || len(idParam) < 1 {
			handleError(w, r, "id not specified")
			return
		}

		badgeId := idParam[0]

		if badgeId != badge {
			var unlocked bool

			switch badgeId {
			case "null":
				unlocked = true
			default:
				tags, err := readPlayerTags(uuid)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				badgeData, err := readPlayerBadgeData(uuid, rank, tags, true, true)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				var badgeFound bool
				for _, badge := range badgeData {
					if badge.BadgeId == badgeId {
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

		if commandParam[0] == "set" {
			err := setPlayerBadge(uuid, badgeId)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		} else {
			rowParam, ok := r.URL.Query()["row"]
			if !ok || len(rowParam) < 1 {
				handleError(w, r, "row not specified")
				return
			}

			colParam, ok := r.URL.Query()["col"]
			if !ok || len(colParam) < 1 {
				handleError(w, r, "col not specified")
				return
			}

			slotRow, err := strconv.Atoi(rowParam[0])
			if err != nil || slotRow < 1 || slotRow > badgeSlotRows {
				handleError(w, r, "invalid row value")
				return
			}

			slotCol, err := strconv.Atoi(colParam[0])
			if err != nil || slotCol < 1 || slotCol > badgeSlotCols {
				handleError(w, r, "invalid col value")
				return
			}

			err = setPlayerBadgeSlot(uuid, badgeId, slotRow, slotCol)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
	case "list":
		var tags []string
		if token != "" {
			var err error
			tags, err = readPlayerTags(uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
		var simple bool
		simpleParam, ok := r.URL.Query()["simple"]
		if ok && len(simpleParam) >= 1 {
			simple = simpleParam[0] == "true"
		}
		if simple {
			simpleBadgeData, err := readSimplePlayerBadgeData(uuid, rank, tags, token != "")
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			simpleBadgeDataJson, err := json.Marshal(simpleBadgeData)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			w.Write([]byte(simpleBadgeDataJson))
		} else {
			if token == "" {
				handleError(w, r, "cannot retrieve player badge data for guest player")
				return
			}
			badgeData, err := readPlayerBadgeData(uuid, rank, tags, true, false)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			badgeDataJson, err := json.Marshal(badgeData)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
			w.Write([]byte(badgeDataJson))
		}
		return
	case "new":
		var tags []string
		if token != "" {
			var err error
			tags, err = readPlayerTags(uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
		newUnlockedBadgeIds, err := readPlayerNewUnlockedBadgeIds(uuid, rank, tags)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if len(newUnlockedBadgeIds) > 0 {
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
		w.Write([]byte(newUnlockedBadgeIdsJson))
		return
	case "slotList":
		badgeSlots, err := readPlayerBadgeSlots(name, badgeSlotRows, badgeSlotCols)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		badgeSlotsJson, err := json.Marshal(badgeSlots)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(badgeSlotsJson))
		return
	case "playerSlotList":
		playerParam, ok := r.URL.Query()["player"]
		if !ok || len(playerParam) < 1 {
			handleError(w, r, "player not specified")
			return
		}

		playerBadgeSlotRows, playerBadgeSlotCols := readPlayerBadgeSlotCounts(playerParam[0])

		badgeSlots, err := readPlayerBadgeSlots(playerParam[0], playerBadgeSlotRows, playerBadgeSlotCols)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		badgeSlotsJson, err := json.Marshal(badgeSlots)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(badgeSlotsJson))
		return
	default:
		handleError(w, r, "unknown command")
		return
	}

	w.Write([]byte("ok"))
}

func handleRanking(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var banned bool

	token := r.Header.Get("Authorization")
	if token == "" {
		uuid, _, banned, _ = readPlayerData(getIp(r))
	} else {
		uuid, _, _, _, banned, _ = readPlayerDataFromToken(token)
	}

	if banned {
		handleError(w, r, "player is banned")
		return
	}

	commandParam, ok := r.URL.Query()["command"]
	if !ok || len(commandParam) < 1 {
		handleError(w, r, "command not specified")
		return
	}

	switch commandParam[0] {
	case "categories":
		rankingCategories, err := readRankingCategories()
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		rankingCategoriesJson, err := json.Marshal(rankingCategories)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		w.Write([]byte(rankingCategoriesJson))
		return
	case "page":
		categoryParam, ok := r.URL.Query()["category"]
		if !ok || len(categoryParam) < 1 {
			handleError(w, r, "category not specified")
			return
		}

		subCategoryParam, ok := r.URL.Query()["subCategory"]
		if !ok || len(subCategoryParam) < 1 {
			handleError(w, r, "subCategory not specified")
			return
		}

		playerPage := 1
		if token != "" {
			var err error
			playerPage, err = readRankingEntryPage(uuid, categoryParam[0], subCategoryParam[0])
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}

		w.Write([]byte(strconv.Itoa(playerPage)))
		return
	case "list":
		categoryParam, ok := r.URL.Query()["category"]
		if !ok || len(categoryParam) < 1 {
			handleError(w, r, "category not specified")
			return
		}

		subCategoryParam, ok := r.URL.Query()["subCategory"]
		if !ok || len(subCategoryParam) < 1 {
			handleError(w, r, "subCategory not specified")
			return
		}

		var page int
		pageParam, ok := r.URL.Query()["page"]
		if !ok || len(pageParam) < 1 {
			page = 1
		} else {
			pageInt, err := strconv.Atoi(pageParam[0])
			if err != nil {
				page = 1
			} else {
				page = pageInt
			}
		}

		rankings, err := readRankingsPaged(categoryParam[0], subCategoryParam[0], page)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		rankingsJson, err := json.Marshal(rankings)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}

		w.Write([]byte(rankingsJson))
		return
	default:
		handleError(w, r, "unknown command")
		return
	}
}

func handleSyncedPics(w http.ResponseWriter, r *http.Request) {
	if syncedPicsResponse == nil {
		response, err := json.Marshal(SyncedPicsInfo{
			PictureNames:    config.pictureNames,
			PicturePrefixes: config.picturePrefixes,
		})
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	
		syncedPicsResponse = response //cache response
	}

	w.Write([]byte(syncedPicsResponse))
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	//GET params user, password
	user, password := r.URL.Query()["user"], r.URL.Query()["password"]
	if len(user) < 1 || len(user[0]) > 12 || !isOkString(user[0]) || len(password) < 1 || len(password[0]) > 72 {
		handleError(w, r, "bad response")
		return
	}

	ip := getIp(r)

	if isVpn(ip) {
		handleError(w, r, "vpn not permitted")
	}

	var userExists int
	db.QueryRow("SELECT COUNT(*) FROM accounts WHERE user = ?", user[0]).Scan(&userExists)

	if userExists > 0 {
		handleError(w, r, "user exists")
		return
	}

	var banned int
	var uuid string
	db.QueryRow("SELECT banned, uuid FROM players WHERE ip = ?", ip).Scan(&uuid, &banned) //no row causes a non-fatal error, uuid is still unset so it doesn't matter

	if banned == 1 {
		handleError(w, r, "banned users cannot create accounts")
		return
	}

	if uuid != "" {
		db.Exec("UPDATE players SET ip = NULL WHERE ip = ?", ip)
	} else {
		uuid, _, _, _ = readPlayerData(ip)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password[0]), bcrypt.DefaultCost)
	if err != nil {
		handleError(w, r, "bcrypt error")
		return
	}

	db.Exec("INSERT INTO accounts (ip, timestampRegistered, uuid, user, pass) VALUES (?, ?, ?, ?, ?)", ip, time.Now(), uuid, user[0], hashedPassword)

	w.Write([]byte("ok"))
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	//GET params user, password
	user, password := r.URL.Query()["user"], r.URL.Query()["password"]
	if len(user) < 1 || !isOkString(user[0]) || len(password) < 1 || len (password[0]) > 72 {
		handleError(w, r, "bad response")
		return
	}

	var userPassHash string
	db.QueryRow("SELECT pass FROM accounts WHERE user = ?", user[0]).Scan(&userPassHash)

	if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password[0])) != nil {
		handleError(w, r, "bad login")
		return
	}

	token := randstr.String(32)
	db.Exec("INSERT INTO playerSessions (sessionId, uuid, expiration) (SELECT ?, uuid, DATE_ADD(NOW(), INTERVAL 30 DAY) FROM accounts WHERE user = ?)", token, user[0])
	db.Exec("UPDATE accounts SET timestampLoggedIn = CURRENT_TIMESTAMP() WHERE user = ?", user[0])

	w.Write([]byte(token))
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")

	if token == "" {
		handleError(w, r, "token not specified")
		return
	}

	uuid, _, _, _, _, _ := readPlayerDataFromToken(token)

	if uuid == "" {
		handleError(w, r, "invalid token")
		return
	}

	db.Exec("DELETE FROM playerSessions WHERE sessionId = ? AND uuid = ?", token, uuid)

	w.Write([]byte("ok"))
}

func handleChangePw(w http.ResponseWriter, r *http.Request) {
	//GET params user, old password, new password
	user, password, newPassword := r.URL.Query()["user"], r.URL.Query()["password"], r.URL.Query()["newPassword"]
	if len(user) < 1 || !isOkString(user[0]) || len(password) < 1 || len(password[0]) > 72 || len(newPassword) < 1 || len(newPassword[0]) > 72 {
		handleError(w, r, "bad response")
		return
	}

	var userPassHash string
	db.QueryRow("SELECT pass FROM accounts WHERE user = ?", user[0]).Scan(&userPassHash)

	if userPassHash == "" || bcrypt.CompareHashAndPassword([]byte(userPassHash), []byte(password[0])) != nil {
		handleError(w, r, "bad login")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword[0]), bcrypt.DefaultCost)
	if err != nil {
		handleError(w, r, "bcrypt error")
		return
	}

	db.Exec("UPDATE accounts SET pass = ? WHERE user = ?", hashedPassword, user[0])

	w.Write([]byte("ok"))
}

func handleResetPw(user string) (newPassword string, err error) {
	var userCount int
	db.QueryRow("SELECT COUNT(*) FROM accounts WHERE user = ?", user).Scan(&userCount)

	if userCount == 0 {
		return "", errors.New("user not found")
	}

	newPassword = randstr.String(8)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.New("bcrypt error")
	}

	db.Exec("UPDATE accounts SET pass = ? WHERE user = ?", hashedPassword, user)

	return newPassword, nil
}

func handleError(w http.ResponseWriter, r *http.Request, payload string) {
	writeErrLog(getIp(r), r.URL.Path, payload)
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(payload))
}

func handleInternalError(w http.ResponseWriter, r *http.Request, err error) {
	writeErrLog(getIp(r), r.URL.Path, err.Error())
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("400 - Bad Request"))
}
