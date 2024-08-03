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
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	globalConditions []*Condition

	conditions             map[string]map[string]*Condition
	badges                 map[string]map[string]*Badge
	badgeUnlockPercentages map[string]float32
	sortedBadgeIds         map[string][]string
)

type Condition struct {
	ConditionId  string   `json:"conditionId"`
	Map          int      `json:"map"`
	MapX1        int      `json:"mapX1"`
	MapY1        int      `json:"mapY1"`
	MapX2        int      `json:"mapX2"`
	MapY2        int      `json:"mapY2"`
	SwitchId     int      `json:"switchId"`
	SwitchValue  bool     `json:"switchValue"`
	SwitchIds    []int    `json:"switchIds"`
	SwitchValues []bool   `json:"switchValues"`
	SwitchDelay  bool     `json:"switchDelay"`
	VarId        int      `json:"varId"`
	VarValue     int      `json:"varValue"`
	VarValue2    int      `json:"varValue2"`
	VarOp        string   `json:"varOp"`
	VarIds       []int    `json:"varIds"`
	VarValues    []int    `json:"varValues"`
	VarOps       []string `json:"varOps"`
	VarDelay     bool     `json:"varDelay"`
	VarTrigger   bool     `json:"varTrigger"`
	Trigger      string   `json:"trigger"`
	Value        string   `json:"value"`
	Values       []string `json:"values"`
	TimeTrial    bool     `json:"timeTrial"`
	Disabled     bool     `json:"disabled"`
}

func (c *Condition) checkSwitch(switchId int, value bool) (bool, int) {
	if switchId == c.SwitchId {
		if c.SwitchValue == value {
			return true, 0
		}
	} else if len(c.SwitchIds) != 0 {
		for s, sId := range c.SwitchIds {
			if switchId == sId {
				if c.SwitchValues[s] == value {
					return true, s
				}
				break
			}
		}
	}

	return false, 0
}

func (c *Condition) checkVar(varId int, value int) (bool, int) {
	if varId == c.VarId {
		var valid bool
		switch c.VarOp {
		case "=":
			valid = value == c.VarValue
		case "<":
			valid = value < c.VarValue
		case ">":
			valid = value > c.VarValue
		case "<=":
			valid = value <= c.VarValue
		case ">=":
			valid = value >= c.VarValue
		case "!=":
			valid = value != c.VarValue
		case ">=<":
			valid = value >= c.VarValue && value < c.VarValue2
		}
		return valid, 0
	} else if len(c.VarIds) != 0 {
		for v, vId := range c.VarIds {
			if varId == vId {
				var valid bool
				switch c.VarOps[v] {
				case "=":
					valid = value == c.VarValues[v]
				case "<":
					valid = value < c.VarValues[v]
				case ">":
					valid = value > c.VarValues[v]
				case "<=":
					valid = value <= c.VarValues[v]
				case ">=":
					valid = value >= c.VarValues[v]
				case "!=":
					valid = value != c.VarValues[v]
				}
				if valid {
					return true, v
				}
				break
			}
		}
	}

	return false, 0
}

type Badge struct {
	Group           string     `json:"group"`
	Order           int        `json:"order"`
	MapOrder        int        `json:"mapOrder"`
	Bp              int        `json:"bp"`
	ReqType         string     `json:"reqType"`
	ReqInt          int        `json:"reqInt"`
	ReqString       string     `json:"reqString"`
	ReqStrings      []string   `json:"reqStrings"`
	ReqStringArrays [][]string `json:"reqStringArrays"`
	ReqCount        int        `json:"reqCount"`
	Map             int        `json:"map"`
	MapX            int        `json:"mapX"`
	MapY            int        `json:"mapY"`
	Secret          bool       `json:"secret"`
	SecretMap       bool       `json:"secretMap"`
	SecretCondition bool       `json:"secretCondition"`
	Hidden          bool       `json:"hidden"`
	Parent          string     `json:"parent"`
	OverlayType     int        `json:"overlayType"`
	Art             string     `json:"art"`
	Animated        bool       `json:"animated"`
	Batch           int        `json:"batch"`
	Dev             bool       `json:"dev"`
}

type SimplePlayerBadge struct {
	BadgeId     string `json:"badgeId"`
	Game        string `json:"game"`
	Group       string `json:"group"`
	Bp          int    `json:"bp"`
	Hidden      bool   `json:"hidden"`
	OverlayType int    `json:"overlayType"`
	Animated    bool   `json:"animated"`
	Unlocked    bool   `json:"unlocked"`
	NewUnlock   bool   `json:"newUnlock"`
}

type PlayerBadge struct {
	BadgeId         string   `json:"badgeId"`
	Game            string   `json:"game"`
	Group           string   `json:"group"`
	Bp              int      `json:"bp"`
	MapId           int      `json:"mapId"`
	MapX            int      `json:"mapX"`
	MapY            int      `json:"mapY"`
	Seconds         int      `json:"seconds"`
	Secret          bool     `json:"secret"`
	SecretCondition bool     `json:"secretCondition"`
	Hidden          bool     `json:"hidden"`
	OverlayType     int      `json:"overlayType"`
	Art             string   `json:"art"`
	Animated        bool     `json:"animated"`
	Percent         float32  `json:"percent"`
	Goals           int      `json:"goals"`
	GoalsTotal      int      `json:"goalsTotal"`
	Tags            []string `json:"tags"`
	Unlocked        bool     `json:"unlocked"`
	NewUnlock       bool     `json:"newUnlock"`
}

type TimeTrialRecord struct {
	MapId   int `json:"mapId"`
	Seconds int `json:"seconds"`
}

func initBadges() {
	setBadgeData()

	scheduler.Every(1).Tuesday().At("20:00").Do(updateActiveBadgesAndConditions)
	scheduler.Every(1).Friday().At("20:00").Do(func() {
		setConditions()
		setBadges()
		globalConditions = getGlobalConditions()
		for _, roomId := range assets.maps {
			rooms[roomId].conditions = getRoomConditions(roomId)
		}
		setBadgeData()
		updateActiveBadgesAndConditions()
	})

	updateActiveBadgesAndConditions()
}

func setBadgeData() {
	if len(badges) == 0 {
		return
	}

	logUpdateTask("badge data")

	badgeUnlockPercentages, _ = getBadgeUnlockPercentages()
	// Use main server to update badge data
	if isMainServer {
		if _, ok := badges[config.gameName]; ok {
			// Badge records needed for determining badge game
			writeGameBadges()
			updatePlayerBadgeSlotCounts("")
		}
	}
}

func updateActiveBadgesAndConditions() {
	logUpdateTask("badge visibility")

	firstBatchDate := time.Date(2022, time.April, 15, 20, 0, 0, 0, time.UTC)
	days := time.Now().UTC().Sub(firstBatchDate).Hours() / 24
	currentBatch := int(math.Floor(days/7)) + 1

	for game, gameBadges := range badges {
		for _, gameBadge := range gameBadges {
			if gameBadge.Batch == 0 {
				continue
			}
			if !gameBadge.Dev {
				gameBadge.Dev = gameBadge.Batch > currentBatch
			}
			switch gameBadge.ReqType {
			case "tag":
				if condition, ok := conditions[game][gameBadge.ReqString]; ok {
					condition.Disabled = gameBadge.Dev
				}
			case "tags":
				for _, tag := range gameBadge.ReqStrings {
					if condition, ok := conditions[game][tag]; ok {
						condition.Disabled = gameBadge.Dev
					}
				}
			case "tagArrays":
				for _, tags := range gameBadge.ReqStringArrays {
					for _, tag := range tags {
						if condition, ok := conditions[game][tag]; ok {
							condition.Disabled = gameBadge.Dev
						}
					}
				}
			}
		}
	}
}

func getGlobalConditions() (globalConditions []*Condition) {
	if gameConditions, ok := conditions[config.gameName]; ok {
		for _, condition := range gameConditions {
			if condition.Map == 0 {
				globalConditions = append(globalConditions, condition)
			}
		}
	}
	return globalConditions
}

func getRoomConditions(roomId int) (roomConditions []*Condition) {
	if gameConditions, ok := conditions[config.gameName]; ok {
		for _, condition := range gameConditions {
			if condition.Map == roomId {
				roomConditions = append(roomConditions, condition)
			}
		}
	}
	return roomConditions
}

// this would probably be better under Room instead of RoomClient
// but passing RoomClient as an argument every time just seems wasteful
// not like anyone's going to see this anyways, right?
func (c *RoomClient) checkRoomConditions(trigger string, value string) {
	if !c.session.account {
		return
	}

	for _, condition := range globalConditions {
		c.checkCondition(condition, 0, nil, trigger, value)
	}

	for _, condition := range c.room.conditions {
		c.checkCondition(condition, c.room.id, c.room.minigames, trigger, value)
	}
}

func (c *RoomClient) checkCondition(condition *Condition, roomId int, minigames []*Minigame, trigger string, value string) {
	if condition.Disabled && c.session.rank < 2 {
		return
	}

	valueMatched := trigger == ""
	if condition.Trigger == trigger && !valueMatched {
		if len(condition.Values) == 0 {
			valueMatched = value == condition.Value
		} else {
			for _, val := range condition.Values {
				if value == val {
					valueMatched = true
					break
				}
			}
		}
	}

	if condition.Trigger == trigger && valueMatched {
		if (condition.SwitchId > 0 || len(condition.SwitchIds) != 0) && !condition.VarTrigger {
			switchId := condition.SwitchId
			if len(condition.SwitchIds) != 0 {
				switchId = condition.SwitchIds[0]
			}
			var switchSyncType int
			if trigger == "" {
				switchSyncType = 2
				if condition.SwitchDelay {
					switchSyncType = 1
				}
			}
			c.outbox <- buildMsg("ss", switchId, switchSyncType)
		} else if condition.VarId > 0 || len(condition.VarIds) != 0 {
			varId := condition.VarId
			if len(condition.VarIds) != 0 {
				varId = condition.VarIds[0]
			}

			if len(minigames) != 0 {
				var skipVarSync bool
				for _, minigame := range minigames {
					if minigame.VarId == varId {
						skipVarSync = true
						break
					}
				}
				if skipVarSync {
					return
				}
			}

			var varSyncType int
			if trigger == "" {
				varSyncType = 2
				if condition.VarDelay {
					varSyncType = 1
				}
			}
			c.outbox <- buildMsg("sv", varId, varSyncType)
		} else if c.checkConditionCoords(condition) {
			timeTrial := condition.TimeTrial && config.gameName == "2kki"
			if !timeTrial {
				success, err := tryWritePlayerTag(c.session.uuid, condition.ConditionId)
				if err != nil {
					writeErrLog(c.session.uuid, c.mapId, err.Error())
				}
				if success {
					c.outbox <- buildMsg("b")
				}
			} else {
				c.outbox <- buildMsg("ss", 1430, 0)
			}
		}
	} else if trigger == "" {
		if condition.Trigger == "event" || condition.Trigger == "eventAction" || condition.Trigger == "picture" {
			var values []string
			if len(condition.Values) == 0 {
				values = append(values, condition.Value)
			} else {
				values = condition.Values
			}
			for _, value := range values {
				if condition.Trigger == "picture" {
					c.outbox <- buildMsg("sp", value)
				} else {
					valueInt, err := strconv.Atoi(value)
					if err != nil {
						writeErrLog(c.session.ip, strconv.Itoa(roomId), err.Error())
						continue
					}

					var eventTriggerType int
					if condition.Trigger == "eventAction" {
						if roomId > 0 && roomId == currentEventVmMapId {
							if eventIds, hasVms := eventVms[roomId]; hasVms {
								var skipEvSync bool
								for _, eventId := range eventIds {
									if eventId != currentEventVmEventId {
										continue
									}
									if valueInt == eventId {
										skipEvSync = true
										break
									}
								}
								if skipEvSync {
									continue
								}
							}
						}

						eventTriggerType = 1
					}

					c.outbox <- buildMsg("sev", value, eventTriggerType)
				}
			}
		} else if condition.Trigger == "coords" {
			c.syncCoords = true
		}
	}
}

func (c *RoomClient) checkConditionCoords(condition *Condition) bool {
	return ((condition.MapX1 <= 0 && condition.MapX2 <= 0) ||
		((condition.MapX1 == -1 || condition.MapX1 <= c.x) && (condition.MapX2 == -1 || condition.MapX2 >= c.x))) &&
		((condition.MapY1 <= 0 && condition.MapY2 <= 0) ||
			((condition.MapY1 == -1 || condition.MapY1 <= c.y) && (condition.MapY2 == -1 || condition.MapY2 >= c.y)))
}

func getPlayerBadgeData(playerUuid string, playerRank int, playerTags []string, account bool, simple bool) (playerBadges []*PlayerBadge, err error) {
	var playerExp int
	var playerEventLocationCount int
	var playerEventLocationCompletion int
	var playerEventVmCount int
	var playerBadgeCount int
	var yume2kkiLocationCompletion int
	var timeTrialRecords []*TimeTrialRecord
	var medalCounts [5]int

	if account {
		playerExp, err = getPlayerTotalEventExp(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		playerEventLocationCount, err = getPlayerEventLocationCount(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		playerEventLocationCompletion, err = getPlayerEventLocationCompletion(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		playerEventVmCount, err = getPlayerEventVmCount(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		yume2kkiLocationCompletion, err = getPlayerGameLocationCompletion(playerUuid, "2kki")
		if err != nil {
			return playerBadges, err
		}
		timeTrialRecords, err = getPlayerTimeTrialRecords(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		medalCounts = getPlayerMedals(playerUuid)
	}

	playerBadgesMap := make(map[string]*PlayerBadge)
	var badgeCountPlayerBadges []*PlayerBadge

	var playerUnlockedBadgeIds []string

	if account {
		playerUnlockedBadgeIds, err = getPlayerUnlockedBadgeIds(playerUuid)
		if err != nil {
			return playerBadges, err
		}
	}

	for game, gameBadges := range badges {
		for badgeId, gameBadge := range gameBadges {
			if gameBadge.Dev && playerRank == 0 {
				continue
			}

			playerBadge := &PlayerBadge{BadgeId: badgeId, Game: game, Group: gameBadge.Group, Bp: gameBadge.Bp, MapId: gameBadge.Map, MapX: gameBadge.MapX, MapY: gameBadge.MapY, Secret: gameBadge.Secret, SecretCondition: gameBadge.SecretCondition, OverlayType: gameBadge.OverlayType, Art: gameBadge.Art, Animated: gameBadge.Animated, Percent: badgeUnlockPercentages[badgeId], Hidden: gameBadge.Hidden || gameBadge.Dev, Tags: []string{}}
			if gameBadge.SecretMap {
				playerBadge.MapId = 0
			}

			if account {
				switch gameBadge.ReqType {
				case "tag":
					for _, tag := range playerTags {
						if tag == gameBadge.ReqString {
							playerBadge.Unlocked = true
							break
						}
					}
				case "tags":
					if gameBadge.ReqCount == 0 || gameBadge.ReqCount >= len(gameBadge.ReqStrings) {
						playerBadge.GoalsTotal = len(gameBadge.ReqStrings)
					} else {
						playerBadge.GoalsTotal = gameBadge.ReqCount
					}
					for _, tag := range playerTags {
						for _, cTag := range gameBadge.ReqStrings {
							if tag == cTag {
								playerBadge.Goals++
								playerBadge.Tags = append(playerBadge.Tags, tag)
								break
							}
						}
					}
				case "tagArrays":
					if gameBadge.ReqCount == 0 || gameBadge.ReqCount >= len(gameBadge.ReqStringArrays) {
						playerBadge.GoalsTotal = len(gameBadge.ReqStringArrays)
					} else {
						playerBadge.GoalsTotal = gameBadge.ReqCount
					}
					for _, cTags := range gameBadge.ReqStringArrays {
						var tagFound bool
						for _, tag := range playerTags {
							for _, cTag := range cTags {
								if tag == cTag {
									tagFound = true
									playerBadge.Goals++
									playerBadge.Tags = append(playerBadge.Tags, tag)
									break
								}
							}
							if tagFound {
								break
							}
						}
					}
				case "exp":
					playerBadge.Goals = playerExp
					playerBadge.GoalsTotal = gameBadge.ReqInt
				case "expCount":
					playerBadge.Goals = playerEventLocationCount
					playerBadge.GoalsTotal = gameBadge.ReqInt
				case "expCompletion":
					playerBadge.Goals = playerEventLocationCompletion
					playerBadge.GoalsTotal = gameBadge.ReqInt
				case "vmCount":
					playerBadge.Goals = playerEventVmCount
					playerBadge.GoalsTotal = gameBadge.ReqInt
				case "badgeCount":
					badgeCountPlayerBadges = append(badgeCountPlayerBadges, playerBadge)
				case "locationCompletion":
					switch game {
					case "2kki":
						playerBadge.Goals = yume2kkiLocationCompletion
						playerBadge.GoalsTotal = gameBadge.ReqInt
					}
				case "timeTrial":
					playerBadge.Seconds = gameBadge.ReqInt
					for _, record := range timeTrialRecords {
						if record.MapId == gameBadge.Map {
							playerBadge.Unlocked = record.Seconds < gameBadge.ReqInt
						}
					}
				case "medal":
					if gameBadge.ReqInt < 5 {
						var medalCount int
						for m := 4; m >= gameBadge.ReqInt; m-- {
							medalCount += medalCounts[m]
						}
						if medalCount > 0 {
							playerBadge.Unlocked = true
						}
					}
				}

				if !playerBadge.Unlocked {
					if playerBadge.GoalsTotal > 0 && playerBadge.Goals >= playerBadge.GoalsTotal {
						playerBadge.Unlocked = true
						playerBadge.Tags = []string{} // no need to do styling anymore
					} else {
						for _, unlockedBadgeId := range playerUnlockedBadgeIds {
							if playerBadge.BadgeId == unlockedBadgeId {
								playerBadge.Unlocked = true
								break
							}
						}
					}
				}
			}

			if playerBadge.Unlocked {
				if !playerBadge.Hidden {
					playerBadgeCount++
				}
			} else if !simple && gameBadge.Hidden && playerRank < 2 {
				continue
			}

			playerBadgesMap[badgeId] = playerBadge
		}

		for _, badgeId := range sortedBadgeIds[game] {
			if playerBadge, ok := playerBadgesMap[badgeId]; ok {
				if playerBadge.Secret {
					if badge, ok := badges[playerBadge.Game][badgeId]; ok {
						parentBadgeId := badge.Parent
						if parentBadgeId != "" {
							playerBadge.Secret = !playerBadgesMap[parentBadgeId].Unlocked
						}
					}
				}

				playerBadges = append(playerBadges, playerBadge)
			}
		}
	}

	var unlockedBadge bool

	for _, badge := range playerBadges {
		if badge.Unlocked {
			var unlocked bool
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
				badge.Percent = badgeUnlockPercentages[badge.BadgeId]
				badge.NewUnlock = true
				unlockedBadge = true
			}
		}
	}

	if unlockedBadge {
		sort.Slice(badgeCountPlayerBadges, func(a, b int) bool {
			playerBadgeA := badgeCountPlayerBadges[a]
			playerBadgeB := badgeCountPlayerBadges[b]

			return badges[playerBadgeA.Game][playerBadgeA.BadgeId].ReqInt < badges[playerBadgeB.Game][playerBadgeB.BadgeId].ReqInt
		})
		for _, playerBadge := range badgeCountPlayerBadges {
			reqBadgeCount := badges[playerBadge.Game][playerBadge.BadgeId].ReqInt
			playerBadge.Goals = playerBadgeCount
			playerBadge.GoalsTotal = reqBadgeCount
			if !playerBadge.Unlocked && playerBadgeCount >= reqBadgeCount {
				playerBadge.Unlocked = true
				err := unlockPlayerBadge(playerUuid, playerBadge.BadgeId)
				if err != nil {
					return playerBadges, err
				}
				playerBadge.Percent = badgeUnlockPercentages[playerBadge.BadgeId]
				playerBadge.NewUnlock = true
			}
		}
	} else if !simple {
		for _, playerBadge := range badgeCountPlayerBadges {
			playerBadge.Goals = playerBadgeCount
			playerBadge.GoalsTotal = badges[playerBadge.Game][playerBadge.BadgeId].ReqInt
		}
	}

	return playerBadges, nil
}

func getSimplePlayerBadgeData(playerUuid string, playerRank int, playerTags []string, account bool) (playerBadges []*SimplePlayerBadge, err error) {
	badgeData, err := getPlayerBadgeData(playerUuid, playerRank, playerTags, account, true)
	if err != nil {
		return playerBadges, err
	}

	for _, badge := range badgeData {
		simpleBadge := &SimplePlayerBadge{BadgeId: badge.BadgeId, Game: badge.Game, Group: badge.Group, Bp: badge.Bp, Hidden: badge.Hidden, OverlayType: badge.OverlayType, Animated: badge.Animated, Unlocked: badge.Unlocked, NewUnlock: badge.NewUnlock}
		playerBadges = append(playerBadges, simpleBadge)
	}

	return playerBadges, nil
}

func getPlayerNewUnlockedBadgeIds(playerUuid string, playerRank int, playerTags []string) (badgeIds []string, err error) {
	badgeData, err := getPlayerBadgeData(playerUuid, playerRank, playerTags, true, true)
	if err != nil {
		return badgeIds, err
	}

	for _, badge := range badgeData {
		if badge.NewUnlock {
			badgeIds = append(badgeIds, badge.BadgeId)
		}
	}

	return badgeIds, nil
}

func setConditions() {
	logUpdateTask("conditions")

	conditionConfig := make(map[string]map[string]*Condition)

	gameConditionDirs, err := os.ReadDir("badges/conditions/")
	if err != nil {
		return
	}

	for _, gameConditionsDir := range gameConditionDirs {
		if gameConditionsDir.IsDir() {
			gameId := gameConditionsDir.Name()
			conditionConfig[gameId] = make(map[string]*Condition)
			configPath := "badges/conditions/" + gameId + "/"
			conditionConfigs, err := os.ReadDir(configPath)
			if err != nil {
				continue
			}

			for _, conditionConfigFile := range conditionConfigs {
				var condition Condition

				data, err := os.ReadFile(configPath + conditionConfigFile.Name())
				if err != nil {
					continue
				}

				err = json.Unmarshal(data, &condition)
				if err == nil {
					conditionId := conditionConfigFile.Name()[:len(conditionConfigFile.Name())-5]
					condition.ConditionId = conditionId
					if condition.VarId > 0 {
						if condition.VarOp == "" {
							condition.VarOp = "="
						}
					} else if len(condition.VarIds) != 0 {
						if len(condition.VarOps) < len(condition.VarIds) {
							for v := range condition.VarIds {
								if v >= len(condition.VarOps) {
									condition.VarOps = append(condition.VarOps, "=")
								}
							}
						}
					}

					conditionConfig[gameId][conditionId] = &condition
				}
			}
		}
	}

	conditions = conditionConfig
}

func setBadges() {
	logUpdateTask("badges")

	badgeConfig := make(map[string]map[string]*Badge)
	sortedBadgeIds = make(map[string][]string)

	gameBadgeDirs, err := os.ReadDir("badges/data/")
	if err != nil {
		return
	}

	for _, gameBadgesDir := range gameBadgeDirs {
		if gameBadgesDir.IsDir() {
			gameId := gameBadgesDir.Name()
			badgeConfig[gameId] = make(map[string]*Badge)
			var badgeIds []string
			configPath := "badges/data/" + gameId + "/"
			badgeConfigs, err := os.ReadDir(configPath)
			if err != nil {
				continue
			}

			for _, badgeConfigFile := range badgeConfigs {
				var badge Badge

				data, err := os.ReadFile(configPath + badgeConfigFile.Name())
				if err != nil {
					continue
				}

				err = json.Unmarshal(data, &badge)
				if err == nil {
					badgeId := badgeConfigFile.Name()[:len(badgeConfigFile.Name())-5]
					badgeConfig[gameId][badgeId] = &badge
					badgeIds = append(badgeIds, badgeId)
				}
			}

			sort.Slice(badgeIds, func(a, b int) bool {
				badgeA := badgeConfig[gameId][badgeIds[a]]
				badgeB := badgeConfig[gameId][badgeIds[b]]

				if badgeA.Group != badgeB.Group {
					return strings.Compare(badgeA.Group, badgeB.Group) == -1
				}

				if badgeA.Order != badgeB.Order {
					return badgeA.Order < badgeB.Order
				} else if badgeA.Map != badgeB.Map {
					sortMapA := badgeA.Map
					sortMapB := badgeB.Map

					if sortMapA == 0 {
						sortMapA = 9999
					} else if sortMapB == 0 {
						sortMapB = 9999
					}

					return sortMapA < sortMapB
				}

				return badgeA.MapOrder < badgeB.MapOrder
			})

			sortedBadgeIds[gameId] = badgeIds
		}
	}

	badges = badgeConfig
}

func getPlayerBadgeSlotCounts(playerName string) (badgeSlotRows int, badgeSlotCols int) {
	err := db.QueryRow("SELECT badgeSlotRows, badgeSlotCols FROM accounts WHERE user = ?", playerName).Scan(&badgeSlotRows, &badgeSlotCols)
	if err != nil {
		return 1, 3
	}

	return badgeSlotRows, badgeSlotCols
}

func updatePlayerBadgeSlotCounts(uuid string) (err error) {
	query := "UPDATE accounts JOIN (SELECT pb.uuid, SUM(b.bp) bp, COUNT(b.badgeId) bc FROM playerBadges pb JOIN badges b ON b.badgeId = pb.badgeId AND b.hidden = 0 GROUP BY pb.uuid) AS pb ON pb.uuid = accounts.uuid " +
		"SET badgeSlotRows = CASE WHEN bp < 300 THEN 1 WHEN bp < 1000 THEN 2 WHEN bp < 2000 THEN 3 WHEN bp < 4000 THEN 4 WHEN bp < 7500 THEN 5 WHEN bp < 12500 THEN 6 WHEN bp < 20000 THEN 7 WHEN bp < 30000 THEN 8 WHEN bp < 50000 THEN 9 ELSE 10 END, " +
		"badgeSlotCols = CASE WHEN bc < 50 THEN 3 WHEN bc < 150 THEN 4 WHEN bc < 300 THEN 5 WHEN bc < 500 THEN 6 ELSE 7 END, " +
		"screenshotLimit = GREATEST(CASE WHEN bp < 100 THEN 10 WHEN bp < 250 THEN 15 WHEN bp < 500 THEN 20 WHEN bp < 1000 THEN 25 WHEN bp < 2500 THEN 30 WHEN bp < 5000 THEN 35 WHEN bp < 7500 THEN 40 WHEN bp < 10000 THEN 45 WHEN bp < 12500 THEN 50 WHEN bp < 15000 THEN 55 WHEN bp < 17500 THEN 60 WHEN bp < 20000 THEN 65 WHEN bp < 25000 THEN 70 ELSE 75 END, screenshotLimit)"
	if uuid == "" {
		_, err = db.Exec(query)
	} else {
		query += " WHERE accounts.uuid = ?"
		_, err = db.Exec(query, uuid)
	}
	if err != nil {
		return err
	}

	return nil
}

func setPlayerBadge(uuid string, badge string) error {
	if client, ok := clients.Load(uuid); ok {
		client.badge = badge
	}

	_, err := db.Exec("UPDATE accounts SET badge = ? WHERE uuid = ?", badge, uuid)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerBadgeSlots(playerName string, badgeSlotRows int, badgeSlotCols int) (badgeSlots [][]string, err error) {
	results, err := db.Query("SELECT pb.badgeId, pb.slotRow, pb.slotCol FROM playerBadges pb JOIN accounts a ON a.uuid = pb.uuid WHERE a.user = ? AND pb.slotRow BETWEEN 1 AND ? AND pb.slotCol BETWEEN 1 AND ? ORDER BY pb.slotRow, pb.slotCol", playerName, badgeSlotRows, badgeSlotCols)
	if err != nil {
		return badgeSlots, err
	}

	defer results.Close()

	var badgeId string
	var badgeRow int
	var badgeCol int

	for r := 1; r <= badgeSlotRows; r++ {
		var badgeSlotRow []string
		for c := 1; c <= badgeSlotCols; c++ {
			if badgeRow > r || (badgeRow == r && badgeCol >= c) {
				if badgeRow == r && badgeCol == c {
					badgeSlotRow = append(badgeSlotRow, badgeId)
				} else {
					badgeSlotRow = append(badgeSlotRow, "null")
				}
			} else {
				for {
					if !results.Next() {
						break
					}
					err := results.Scan(&badgeId, &badgeRow, &badgeCol)
					if err != nil {
						break
					}

					if badgeRow > r || (badgeRow == r && badgeCol >= c) {
						if badgeRow == r && badgeCol == c {
							badgeSlotRow = append(badgeSlotRow, badgeId)
						}
						break
					}
				}
				if len(badgeSlotRow) < c {
					badgeSlotRow = append(badgeSlotRow, "null")
				}
			}
		}
		badgeSlots = append(badgeSlots, badgeSlotRow)
	}

	return badgeSlots, nil
}

func setPlayerBadgeSlot(uuid string, badgeId string, slotRow int, slotCol int) error {
	var slotCurrentBadgeId string
	err := db.QueryRow("SELECT badgeId FROM playerBadges WHERE uuid = ? AND slotRow = ? AND slotCol = ? LIMIT 1", uuid, slotRow, slotCol).Scan(&slotCurrentBadgeId)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	} else if slotCurrentBadgeId == badgeId {
		return nil
	} else {
		if badgeId != "null" {
			var badgeCurrentSlotRow, badgeCurrentSlotCol int
			err := db.QueryRow("SELECT slotRow, slotCol FROM playerBadges WHERE uuid = ? AND badgeId = ? LIMIT 1", uuid, badgeId).Scan(&badgeCurrentSlotRow, &badgeCurrentSlotCol)
			if err != nil && err != sql.ErrNoRows {
				return err
			} else {
				_, err = db.Exec("UPDATE playerBadges SET slotRow = ?, slotCol = ? WHERE uuid = ? AND badgeId = ?", badgeCurrentSlotRow, badgeCurrentSlotCol, uuid, slotCurrentBadgeId)
				if err != nil && err != sql.ErrNoRows {
					return err
				}
			}
		} else {
			_, err = db.Exec("UPDATE playerBadges SET slotRow = 0, slotCol = 0 WHERE uuid = ? AND slotRow = ? AND slotCol = ?", uuid, slotRow, slotCol)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
		}
	}

	_, err = db.Exec("UPDATE playerBadges SET slotRow = ?, slotCol = ? WHERE uuid = ? AND badgeId = ?", slotRow, slotCol, uuid, badgeId)
	if err != nil {
		return err
	}

	return nil
}

func writeGameBadges() error {
	_, err := db.Exec("TRUNCATE TABLE badges")
	if err != nil {
		return err
	}

	for badgeGame := range badges {
		for badgeId, badge := range badges[badgeGame] {
			if _, ok := badges[config.gameName]; ok {
				badgeUnlockPercentage := badgeUnlockPercentages[badgeId]
				_, err = db.Exec("INSERT INTO badges (badgeId, game, bp, hidden, percentUnlocked) VALUES (?, ?, ?, ?, ?)", badgeId, badgeGame, badge.Bp, badge.Hidden || badge.Dev, badgeUnlockPercentage)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func getPlayerUnlockedBadgeIds(playerUuid string) (unlockedBadgeIds []string, err error) {
	results, err := db.Query("SELECT badgeId FROM playerBadges WHERE uuid = ?", playerUuid)
	if err != nil {
		return unlockedBadgeIds, err
	}

	defer results.Close()

	for results.Next() {
		var badgeId string
		err := results.Scan(&badgeId)
		if err != nil {
			return unlockedBadgeIds, err
		}
		unlockedBadgeIds = append(unlockedBadgeIds, badgeId)
	}

	return unlockedBadgeIds, nil
}

func unlockPlayerBadge(playerUuid string, badgeId string) error {
	_, err := db.Exec("INSERT INTO playerBadges (uuid, badgeId, timestampUnlocked) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE badgeId = badgeId", playerUuid, badgeId, time.Now())
	if err != nil {
		return err
	}

	badgeUnlockPercentages[badgeId], err = getBadgeUnlockPercentage(badgeId)
	if err != nil {
		return err
	}

	return nil
}

func removePlayerBadge(playerUuid string, badgeId string) error {
	var slotRow int
	var slotCol int

	err := db.QueryRow("SELECT slotRow, slotCol FROM playerBadges WHERE uuid = ? AND badgeId = ?", playerUuid, badgeId).Scan(&slotRow, &slotCol)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM playerBadges WHERE uuid = ? AND badgeId = ?", playerUuid, badgeId)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE accounts SET badge = 'null' WHERE uuid = ? AND badge = ?", playerUuid, badgeId)
	if err != nil {
		return err
	}

	return nil
}

func getBadgeUnlockPercentage(badgeId string) (unlockPercentage float32, err error) {
	err = db.QueryRow("SELECT COALESCE(COUNT(b.uuid) / aa.count, 0) * 100 FROM playerBadges b JOIN accounts a ON a.uuid = b.uuid JOIN (SELECT COUNT(aa.uuid) count FROM accounts aa WHERE EXISTS(SELECT * FROM playerBadges aab WHERE aab.uuid = aa.uuid AND aa.inactive = 0)) aa WHERE EXISTS(SELECT * FROM playerBadges ab WHERE ab.uuid = a.uuid AND a.inactive = 0) AND b.badgeId = ?", badgeId).Scan(&unlockPercentage)

	return unlockPercentage, err
}

func getBadgeUnlockPercentages() (unlockPercentages map[string]float32, err error) {
	results, err := db.Query("SELECT b.badgeId, (COUNT(b.uuid) / aa.count) * 100 FROM playerBadges b JOIN accounts a ON a.uuid = b.uuid JOIN (SELECT COUNT(aa.uuid) count FROM accounts aa WHERE EXISTS(SELECT * FROM playerBadges aab WHERE aab.uuid = aa.uuid AND aa.inactive = 0)) aa WHERE EXISTS(SELECT * FROM playerBadges ab WHERE ab.uuid = a.uuid AND a.inactive = 0) GROUP BY b.badgeId")
	if err != nil {
		return unlockPercentages, err
	}

	defer results.Close()

	unlockPercentages = make(map[string]float32)

	for results.Next() {
		var badgeId string
		var percentUnlocked float32

		err := results.Scan(&badgeId, &percentUnlocked)
		if err != nil {
			return unlockPercentages, err
		}

		unlockPercentages[badgeId] = percentUnlocked
	}

	return unlockPercentages, nil
}
