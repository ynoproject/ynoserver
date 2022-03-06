package server

import (
	"database/sql"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"log"
	"errors"
	"strings"
	"github.com/thanhpk/randstr"
	"strconv"
)

type HubController struct {
	hubs []*Hub
	config Config

	database *sql.DB
}

func (h *HubController) addHub(roomName string) {
	hub := NewHub(roomName, h)
	h.hubs = append(h.hubs, hub)
	go hub.Run()
}

func (h *HubController) isVpn(ip string) (bool, error) {
	apiKey := ""

	if apiKey == "" {
		return false, nil //VPN checking is not available
	}

	req, err := http.NewRequest("GET", "http://v2.api.iphub.info/ip/" + ip, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("X-Key", apiKey)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var response IpHubResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return false, err
	}

	var blockedIp bool
	if response.Block == 0 {
		blockedIp = false
	} else {
		blockedIp = true
	}
	
	if response.Block > 0 {
		log.Printf("Connection Blocked %v %v %v %v\n", response.IP, response.CountryName, response.Isp, response.Block)
		return false, errors.New("connection banned")
	}

	return blockedIp, nil
}

func (h *HubController) globalBroadcast(inpData []byte) {
	for _, hub := range h.hubs {
		hub.broadcast(inpData)
	}
}

func (h *HubController) isValidSpriteName(name string) bool {
	if name == "" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range h.config.spriteNames {
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func (h *HubController) isValidSystemName(name string) bool {
	for _, otherName := range h.config.systemNames {
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func (h *HubController) isValidSoundName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range h.config.soundNames {
		if strings.EqualFold(otherName, name) {
			for _, ignoredName := range h.config.ignoredSoundNames {
				if strings.EqualFold(ignoredName, name) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func (h *HubController) isValidPicName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	nameLower := strings.ToLower(name)
	for _, otherName := range h.config.pictureNames {
		if otherName == nameLower {
			return true
		}
	}
	for _, prefix := range h.config.picturePrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return true
		}
	}

	return false
}

func (h *HubController) readPlayerData(ip string) (uuid string, rank int, banned bool) {
	results, err := h.queryDatabase("SELECT uuid, rank, banned FROM playerdata WHERE ip = '" + ip + "'")
	if err != nil {
		return "", 0, false
	}
	
	defer results.Close()

	if results.Next() {
		err := results.Scan(&uuid, &rank, &banned)
		if err != nil {
			return "", 0, false
		}
	} else {
		uuid = randstr.String(16)
		banned, _ := h.isVpn(ip)
		h.createPlayerData(ip, uuid, 0, banned)
	} 

	return uuid, rank, banned
}

func (h *HubController) readPlayerRank(uuid string) (rank int) {
	results, err := h.queryDatabase("SELECT rank FROM playerdata WHERE uuid = '" + uuid + "'")
	if err != nil {
		return 0
	}
	
	defer results.Close()

	if results.Next() {
		err := results.Scan(&rank)
		if err != nil {
			return 0
		}
	}

	return rank
}

func (h *HubController) tryBanPlayer(senderIp, uuid string) error {
	senderUUID, senderRank, _ := h.readPlayerData(senderIp)
	if senderUUID == uuid {
		return errors.New("attempted self-ban")
	}
	rank := h.readPlayerRank(uuid)
	if senderRank <= rank {
		return errors.New("unauthorized ban")
	}

	results, err := h.queryDatabase("UPDATE playerdata SET banned = true WHERE uuid = '" + uuid + "'")
	if err != nil {
		return err
	}
	
	defer results.Close()

	return nil
}

func (h *HubController) createPlayerData(ip string, uuid string, rank int, banned bool) error {
	results, err := h.queryDatabase("INSERT INTO playerdata (ip, uuid, rank, banned) VALUES ('" + ip + "', '" + uuid + "', " + strconv.Itoa(rank) + ", " + strconv.FormatBool(banned) + ") ON DUPLICATE KEY UPDATE uuid = '" + uuid + "', rank = " + strconv.Itoa(rank) + ", banned = " + strconv.FormatBool(banned))
	if err != nil {
		return err
	}
	
	defer results.Close()

	return nil
}

func (h *HubController) queryDatabase(query string) (*sql.Rows, error) {
	if h.database == nil {
		return nil, nil
	}

	results, err := h.database.Query(query)
	if err != nil {
		return nil, err
	}

	return results, err
}
