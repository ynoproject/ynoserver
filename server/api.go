package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type Party struct {
	Id          int           `json:"id"`
	Name        string        `json:"name"`
	Public      bool          `json:"public"`
	SystemName  string        `json:"systemName"`
	Description string        `json:"description"`
	OwnerUuid   string        `json:"ownerUuid"`
	Members     []PartyMember `json:"members"`
}

type PartyMember struct {
	Uuid          string `json:"uuid"`
	Name          string `json:"name"`
	Rank          int    `json:"rank"`
	SystemName    string `json:"systemName"`
	SpriteName    string `json:"spriteName"`
	SpriteIndex   int    `json:"spriteIndex"`
	MapId         string `json:"mapId"`
	PrevMapId     string `json:"prevMapId"`
	PrevLocations string `json:"prevLocations"`
	Online        bool   `json:"online"`
}

func StartApi() {
	http.HandleFunc("/api/admin", handleAdmin)
	http.HandleFunc("/api/party", handleParty)
	http.HandleFunc("/api/ploc", handlePloc)

	http.HandleFunc("/api/players", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strconv.Itoa(len(allClients))))
	})
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	uuid, rank, _ := readPlayerData(r.Header.Get("x-forwarded-for"))
	if rank == 0 {
		handleError(w, r, "access denied")
		return
	}

	commandParam, ok := r.URL.Query()["command"]
	if !ok || len(commandParam) < 1 {
		handleError(w, r, "command not specified")
		return
	}

	if commandParam[0] == "ban" {
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
	} else {
		handleError(w, r, "unknown command")
		return
	}

	w.Write([]byte("ok"))
}

func handleParty(w http.ResponseWriter, r *http.Request) {
	ip := r.Header.Get("x-forwarded-for")
	uuid, rank, banned := readPlayerData(ip)

	if banned {
		handleError(w, r, "player is banned")
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
		partyListData, err := readAllPartyData(rank < 1, uuid)
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
		partyData, err := readPartyData(partyId, uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		partyDataJson, err := json.Marshal(partyData)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(partyDataJson))
		return
	case "create":
		partyId, err := readPlayerPartyId(uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		if partyId > 0 {
			handleError(w, r, "player already in a party")
			return
		}
		nameParam, ok := r.URL.Query()["name"]
		if !ok || len(nameParam) < 1 {
			handleError(w, r, "name not specified")
			return
		}
		if len(nameParam) > 255 {
			handleError(w, r, "name too long")
		}
		var public bool
		publicParam, ok := r.URL.Query()["public"]
		if ok && len(publicParam) >= 1 {
			public = true
		}
		themeParam, ok := r.URL.Query()["theme"]
		if !ok || len(themeParam) < 1 {
			handleError(w, r, "theme not specified")
			return
		}
		if !isValidSystemName(themeParam[0]) {
			handleError(w, r, "invalid system name for theme")
		}
		partyId, err = createPartyData(nameParam[0], public, themeParam[0], "", uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		err = writePlayerParty(partyId, uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
		w.Write([]byte(strconv.Itoa(partyId)))
		return
	case "join":
		partyIdParam, ok := r.URL.Query()["partyId"]
		if !ok || len(partyIdParam) < 1 {
			handleError(w, r, "partyId not specified")
			return
		}
		partyId, err := strconv.Atoi(partyIdParam[0])
		if err != nil {
			handleError(w, r, "invalid partyId value")
		}
		err = writePlayerParty(partyId, uuid)
		if err != nil {
			handleInternalError(w, r, err)
			return
		}
	case "leave":
		err := handlePartyMemberLeave(uuid)
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

func handlePartyMemberLeave(playerUuid string) error {
	partyId, err := readPlayerPartyId(playerUuid)
	if err != nil {
		return err
	}

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

func handlePloc(w http.ResponseWriter, r *http.Request) {
	uuid, _, _ := readPlayerData(r.Header.Get("x-forwarded-for"))

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
	} else {
		handleError(w, r, "client not found")
		return
	}

	w.Write([]byte("ok"))
}

func handleError(w http.ResponseWriter, r *http.Request, payload string) {
	writeErrLog(r.Header.Get("x-forwarded-for"), r.URL.Path, payload)
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(payload))
}

func handleInternalError(w http.ResponseWriter, r *http.Request, err error) {
	writeErrLog(r.Header.Get("x-forwarded-for"), r.URL.Path, err.Error())
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("400 - Bad Request"))
}
