package server

import (
	"encoding/json"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
)

type Badge struct {
	BadgeId   string `json:"badgeId"`
	Order     int    `json:"order"`
	ReqType   string `json:"reqType"`
	ReqString string `json:"reqString"`
	ReqInt    int    `json:"reqInt"`
	Map       int    `json:"map"`
	MapX      int    `json:"mapX"`
	MapY      int    `json:"mapY"`
	Secret    bool   `json:"secret"`
	Parent    string `json:"parent"`
	Overlay   bool   `json:"overlay"`
	Dev       bool   `json:"dev"`
}

type PlayerBadge struct {
	BadgeId   string  `json:"badgeId"`
	Game      string  `json:"game"`
	MapId     int     `json:"mapId"`
	MapX      int     `json:"mapX"`
	MapY      int     `json:"mapY"`
	Seconds   int     `json:"seconds"`
	Secret    bool    `json:"secret"`
	Overlay   bool    `json:"overlay"`
	Percent   float64 `json:"percent"`
	Unlocked  bool    `json:"unlocked"`
	NewUnlock bool    `json:"newUnlock"`
}

type Condition struct {
	ConditionId string `json:"conditionId"`
	Map         int    `json:"map"`
	SwitchId    int    `json:"switchId"`
	SwitchValue bool   `json:"switchValue"`
	SwitchDelay bool   `json:"switchDelay"`
	VarId       int    `json:"varId"`
	VarValue    int    `json:"varValue"`
	VarDelay    bool   `json:"varDelay"`
	Trigger     string `json:"trigger"`
	Value       string `json:"value"`
	TimeTrial   bool   `json:"timeTrial"`
}

func getHubConditions(roomName string) (hubConditions []*Condition) {
	if _, ok := conditions[config.gameName]; ok {
		mapId, _ := strconv.Atoi(roomName)
		for _, condition := range conditions[config.gameName] {
			if condition.Map == mapId {
				hubConditions = append(hubConditions, condition)
			}
		}
	}
	return hubConditions
}

func checkHubConditions(h *Hub, client *Client, trigger string, value string) {
	if !client.account {
		return
	}
	for _, c := range h.conditions {
		if !c.TimeTrial {
			if c.Trigger == trigger && (trigger == "" || value == c.Value) {
				if c.SwitchId > 0 {
					switchSyncType := 2
					if c.SwitchDelay {
						switchSyncType = 1
					}
					client.send <- []byte("ss" + paramDelimStr + strconv.Itoa(c.SwitchId) + paramDelimStr + strconv.Itoa(switchSyncType))
				} else if c.VarId > 0 {
					varSyncType := 2
					if c.VarDelay {
						varSyncType = 1
					}
					client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(c.VarId) + paramDelimStr + strconv.Itoa(varSyncType))
				} else {
					_, err := tryWritePlayerTag(client.uuid, c.ConditionId)
					if err != nil {
						writeErrLog(client.ip, h.roomName, err.Error())
					}
				}
			}
		} else if config.gameName == "2kki" {
			if c.SwitchId > 0 {
				switchSyncType := 2
				if c.SwitchDelay {
					switchSyncType = 1
				}
				client.send <- []byte("ss" + paramDelimStr + strconv.Itoa(c.SwitchId) + paramDelimStr + strconv.Itoa(switchSyncType))
			} else if c.VarId > 0 {
				varSyncType := 2
				if c.VarDelay {
					varSyncType = 1
				}
				client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(c.VarId) + paramDelimStr + strconv.Itoa(varSyncType))
			} else {
				client.send <- []byte("sv" + paramDelimStr + "88" + paramDelimStr + "0")
			}
		}
	}
}

func readPlayerBadgeData(playerUuid string, playerRank int, playerTags []string) (playerBadges []*PlayerBadge, err error) {
	playerExp, err := readPlayerTotalEventExp(playerUuid)
	if err != nil {
		return playerBadges, err
	}
	playerEventLocationCompletion, err := readPlayerEventLocationCompletion(playerUuid)
	if err != nil {
		return playerBadges, err
	}
	timeTrialRecords, err := readPlayerTimeTrialRecords(playerUuid)
	if err != nil {
		return playerBadges, err
	}

	playerBadgesMap := make(map[string]*PlayerBadge)

	for game, gameBadges := range badges {
		for badgeId, gameBadge := range gameBadges {
			if gameBadge.Dev && playerRank < 2 {
				continue
			}
			playerBadge := &PlayerBadge{BadgeId: badgeId, Game: game, MapId: gameBadge.Map, MapX: gameBadge.MapX, MapY: gameBadge.MapY, Secret: gameBadge.Secret, Overlay: gameBadge.Overlay}
			switch gameBadge.ReqType {
			case "tag":
				for _, tag := range playerTags {
					if tag == gameBadge.ReqString {
						playerBadge.Unlocked = true
						break
					}
				}
			case "exp":
				playerBadge.Unlocked = playerExp >= gameBadge.ReqInt
			case "expCompletion":
				playerBadge.Unlocked = playerEventLocationCompletion >= gameBadge.ReqInt
			case "timeTrial":
				for _, record := range timeTrialRecords {
					if record.MapId == gameBadge.Map {
						playerBadge.Unlocked = record.Seconds < gameBadge.ReqInt
						playerBadge.Seconds = gameBadge.ReqInt
					}
				}
			}

			playerBadgesMap[badgeId] = playerBadge
		}
	}

	for badgeId, playerBadge := range playerBadgesMap {
		if playerBadge.Secret {
			if _, ok := badges[playerBadge.Game][badgeId]; ok {
				parentBadgeId := badges[playerBadge.Game][badgeId].Parent
				if parentBadgeId != "" {
					playerBadge.Secret = !playerBadgesMap[parentBadgeId].Unlocked
				}
			}
		}

		playerBadges = append(playerBadges, playerBadge)
	}

	sort.Slice(playerBadges, func(a, b int) bool {
		playerBadgeA := playerBadges[a]
		playerBadgeB := playerBadges[b]
		if playerBadgeA.Game != playerBadgeB.Game {
			return strings.Compare(playerBadgeA.Game, playerBadgeB.Game) == -1
		}
		return badges[playerBadgeA.Game][playerBadgeA.BadgeId].Order < badges[playerBadgeB.Game][playerBadgeB.BadgeId].Order
	})

	playerUnlockedBadgeIds, err := readPlayerUnlockedBadgeIds(playerUuid)
	if err != nil {
		return playerBadges, err
	}

	unlockPercentages, err := readBadgeUnlockPercentages()
	if err != nil {
		return playerBadges, err
	}

	for _, badge := range playerBadges {
		for _, badgePercentUnlocked := range unlockPercentages {
			if badge.BadgeId == badgePercentUnlocked.BadgeId {
				badge.Percent = badgePercentUnlocked.Percent
				break
			}
		}

		if badge.Unlocked {
			unlocked := false
			for _, unlockedBadgeId := range playerUnlockedBadgeIds {
				if badge.BadgeId == unlockedBadgeId {
					unlocked = true
					break
				}
			}
			if !unlocked {
				err := unlockPlayerBadge(playerUuid, badge.BadgeId)
				if err != nil {
					return playerBadges, err
				}
				badge.NewUnlock = true
			}
		} else {
			for _, unlockedBadgeId := range playerUnlockedBadgeIds {
				if badge.BadgeId == unlockedBadgeId {
					badge.Unlocked = true
					break
				}
			}
		}
	}

	return playerBadges, nil
}

func SetConditions() {
	conditionConfig := make(map[string]map[string]*Condition)

	gameConditionDirs, err := ioutil.ReadDir("badges/conditions/")
	if err != nil {
		return
	}

	for _, gameConditionsDir := range gameConditionDirs {
		if gameConditionsDir.IsDir() {
			gameId := gameConditionsDir.Name()
			conditionConfig[gameId] = make(map[string]*Condition)
			configPath := "badges/conditions/" + gameId + "/"
			conditionConfigs, err := ioutil.ReadDir(configPath)
			if err != nil {
				continue
			}

			for _, conditionConfigFile := range conditionConfigs {
				condition := &Condition{}

				data, err := ioutil.ReadFile(configPath + conditionConfigFile.Name())
				if err != nil {
					continue
				}

				err = json.Unmarshal(data, &condition)
				if err == nil {
					conditionId := conditionConfigFile.Name()[:len(conditionConfigFile.Name())-5]
					condition.ConditionId = conditionId
					conditionConfig[gameId][conditionId] = condition
				}
			}
		}
	}

	conditions = conditionConfig
}

func SetBadges() {
	badgeConfig := make(map[string]map[string]*Badge)

	gameBadgeDirs, err := ioutil.ReadDir("badges/data/")
	if err != nil {
		return
	}

	for _, gameBadgesDir := range gameBadgeDirs {
		if gameBadgesDir.IsDir() {
			gameId := gameBadgesDir.Name()
			badgeConfig[gameId] = make(map[string]*Badge)
			configPath := "badges/data/" + gameId + "/"
			badgeConfigs, err := ioutil.ReadDir(configPath)
			if err != nil {
				continue
			}

			for _, badgeConfigFile := range badgeConfigs {
				badge := &Badge{}

				data, err := ioutil.ReadFile(configPath + badgeConfigFile.Name())
				if err != nil {
					continue
				}

				err = json.Unmarshal(data, &badge)
				if err == nil {
					badgeId := badgeConfigFile.Name()[:len(badgeConfigFile.Name())-5]
					badgeConfig[gameId][badgeId] = badge
				}
			}
		}
	}

	badges = badgeConfig
}
