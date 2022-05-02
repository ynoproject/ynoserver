package main

import (
	"database/sql"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type PlayerInfo struct {
	Uuid  string `json:"uuid"`
	Name  string `json:"name"`
	Rank  int    `json:"rank"`
	Badge string `json:"badge"`
}

type Party struct {
	Id          int           `json:"id"`
	Name        string        `json:"name"`
	Public      bool          `json:"public"`
	Pass        string        `json:"pass"`
	SystemName  string        `json:"systemName"`
	Description string        `json:"description"`
	OwnerUuid   string        `json:"ownerUuid"`
	Members     []PartyMember `json:"members"`
}

type PartyMember struct {
	Uuid          string `json:"uuid"`
	Name          string `json:"name"`
	Rank          int    `json:"rank"`
	Account       bool   `json:"account"`
	Badge         string `json:"badge"`
	SystemName    string `json:"systemName"`
	SpriteName    string `json:"spriteName"`
	SpriteIndex   int    `json:"spriteIndex"`
	MapId         string `json:"mapId"`
	PrevMapId     string `json:"prevMapId"`
	PrevLocations string `json:"prevLocations"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	Online        bool   `json:"online"`
}

func StartApi() {
	http.HandleFunc("/api/admin", handleAdmin)
	http.HandleFunc("/api/party", handleParty)
	http.HandleFunc("/api/saveSync", handleSaveSync)
	http.HandleFunc("/api/eventLocations", handleEventLocations)
	http.HandleFunc("/api/badge", handleBadge)
	http.HandleFunc("/api/ranking", handleRanking)
	http.HandleFunc("/api/ploc", handlePloc)

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

		result := db.QueryRow("SELECT response FROM 2kkiApiQueries WHERE action = ? AND query = ?", actionParam[0], queryString)
		err := result.Scan(&response)

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

				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}

				if strings.HasPrefix(string(body), "{\"error\"") || strings.HasPrefix(string(body), "<!DOCTYPE html>") {
					writeErrLog(getIp(r), r.URL.Path, "received error response from Yume 2kki Explorer API: "+string(body))
				} else {
					_, err = db.Exec("INSERT INTO 2kkiApiQueries (action, query, response) VALUES (?, ?, ?)", actionParam[0], queryString, string(body))
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

		session := r.Header.Get("X-Session")
		if session == "" {
			uuid, name, rank = readPlayerInfo(getIp(r))
		} else {
			uuid, name, rank, badge = readPlayerInfoFromSession(session)
		}
		playerInfo := PlayerInfo{
			Uuid:  uuid,
			Name:  name,
			Rank:  rank,
			Badge: badge,
		}
		playerInfoJson, err := json.Marshal(playerInfo)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(playerInfoJson))
	})
	http.HandleFunc("/api/players", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strconv.Itoa(len(allClients))))
	})
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var rank int

	session := r.Header.Get("X-Session")
	if session == "" {
		uuid, rank, _ = readPlayerData(getIp(r))
	} else {
		uuid, _, rank, _, _ = readPlayerDataFromSession(session)
	}
	if rank == 0 {
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

	session := r.Header.Get("X-Session")
	if session == "" {
		uuid, rank, banned = readPlayerData(getIp(r))
	} else {
		uuid, _, rank, _, banned = readPlayerDataFromSession(session)
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
		partyListData, err := readAllPartyData()
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
	case "get":
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
		partyData, err := readPartyData(uuid)
		if err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("401 - Unauthorized"))
				return
			}
			handleInternalError(w, r, err)
			return
		}
		if partyData.Id != partyId {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("401 - Unauthorized"))
			return
		}
		if uuid != partyData.OwnerUuid {
			partyData.Pass = ""
		}
		partyDataJson, err := json.Marshal(partyData)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(partyDataJson))
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
		if !isValidSystemName(themeParam[0], true) {
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

	session := r.Header.Get("X-Session")
	if session == "" {
		handleError(w, r, "session token not specified")
		return
	} else {
		uuid, _, _, _, banned = readPlayerDataFromSession(session)
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
		data, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
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

func handleEventLocations(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var banned bool

	session := r.Header.Get("X-Session")
	if session == "" {
		handleError(w, r, "session token not specified")
		return
	} else {
		uuid, _, _, _, banned = readPlayerDataFromSession(session)
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
	case "period":
		period, err := readCurrentEventPeriodData()
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		periodJson, err := json.Marshal(period)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(periodJson))
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
	case "list":
		periodId, err := readCurrentEventPeriodId()
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		currentEventLocationsData, err := readCurrentPlayerEventLocationsData(periodId, uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		hasIncompleteEvent := false
		for _, currentEventLocation := range currentEventLocationsData {
			if !currentEventLocation.Complete {
				hasIncompleteEvent = true
				break
			}
		}
		if !hasIncompleteEvent && config.gameName == "2kki" {
			add2kkiEventLocationsWithExp(-1, 1, 0, uuid)
			currentEventLocationsData, err = readCurrentPlayerEventLocationsData(periodId, uuid)
			if err != nil {
				handleInternalError(w, r, err)
				return
			}
		}
		currentEventLocationsDataJson, err := json.Marshal(currentEventLocationsData)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(currentEventLocationsDataJson))
	case "claim":
		locationParam, ok := r.URL.Query()["location"]
		if !ok || len(locationParam) < 1 {
			handleError(w, r, "location not specified")
			return
		}
		free := false
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
		if _, found := allClients[uuid]; found {
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
			hasIncompleteEvent := false
			for _, currentEventLocation := range currentEventLocationsData {
				if !currentEventLocation.Complete {
					hasIncompleteEvent = true
					break
				}
			}
			if !hasIncompleteEvent && config.gameName == "2kki" {
				add2kkiEventLocationsWithExp(-1, 1, 0, uuid)
			}
		} else {
			handleError(w, r, "unexpected state")
			return
		}
		w.Write([]byte(strconv.Itoa(ret)))
	default:
		handleError(w, r, "unknown command")
	}
}

func handleBadge(w http.ResponseWriter, r *http.Request) {
	var uuid string
	var rank int
	var badge string
	var banned bool

	session := r.Header.Get("X-Session")
	if session == "" {
		handleError(w, r, "session token not specified")
		return
	} else {
		uuid, _, rank, badge, banned = readPlayerDataFromSession(session)
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
	case "set":
		idParam, ok := r.URL.Query()["id"]
		if !ok || len(idParam) < 1 {
			handleError(w, r, "id not specified")
			return
		}

		badgeId := idParam[0]

		if badgeId != badge {
			unlocked := false

			switch badgeId {
			case "null":
				unlocked = true
			default:
				tags, err := readPlayerTags(uuid)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				badgeData, err := readPlayerBadgeData(uuid, rank, tags)
				if err != nil {
					handleInternalError(w, r, err)
					return
				}
				badgeFound := false
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

		setPlayerBadge(uuid, badgeId)
	case "list":
		tags, err := readPlayerTags(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		badgeData, err := readPlayerBadgeData(uuid, rank, tags)
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

	session := r.Header.Get("X-Session")
	if session == "" {
		uuid, _, banned = readPlayerData(getIp(r))
	} else {
		uuid, _, _, _, banned = readPlayerDataFromSession(session)
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
		if session != "" {
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

func handlePloc(w http.ResponseWriter, r *http.Request) {
	var uuid string

	session := r.Header.Get("X-Session")
	if session == "" {
		uuid, _, _ = readPlayerData(getIp(r))
	} else {
		uuid, _, _, _, _ = readPlayerDataFromSession(session)
	}

	prevMapIdParam, ok := r.URL.Query()["prevMapId"]
	if !ok || len(prevMapIdParam) < 1 {
		handleError(w, r, "prevMapId not specified")
		return
	}

	if len(prevMapIdParam[0]) != 4 {
		handleError(w, r, "invalid prevMapId")
		return
	}

	prevLocationsParam, ok := r.URL.Query()["prevLocations"]
	if !ok {
		handleError(w, r, "prevLocations not specified")
		return
	}

	if client, found := allClients[uuid]; found {
		client.prevMapId = prevMapIdParam[0]
		if len(prevLocationsParam) > 0 {
			client.prevLocations = prevLocationsParam[0]
		} else {
			client.prevLocations = ""
		}
		checkHubConditions(client.hub, client, "prevMap", client.prevMapId)
	} else {
		handleError(w, r, "client not found")
		return
	}

	w.Write([]byte("ok"))
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
