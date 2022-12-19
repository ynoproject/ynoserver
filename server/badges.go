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

	conditions     map[string]map[string]*Condition
	badges         map[string]map[string]*Badge
	sortedBadgeIds map[string][]string
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

func (c Condition) checkSwitch(switchId int, value bool) (bool, int) {
	if switchId == c.SwitchId {
		if c.SwitchValue == value {
			return true, 0
		}
	} else if len(c.SwitchIds) > 0 {
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

func (c Condition) checkVar(varId int, value int) (bool, int) {
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
	} else if len(c.VarIds) > 0 {
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
	BadgeId         string  `json:"badgeId"`
	Game            string  `json:"game"`
	Group           string  `json:"group"`
	Bp              int     `json:"bp"`
	MapId           int     `json:"mapId"`
	MapX            int     `json:"mapX"`
	MapY            int     `json:"mapY"`
	Seconds         int     `json:"seconds"`
	Secret          bool    `json:"secret"`
	SecretCondition bool    `json:"secretCondition"`
	Hidden          bool    `json:"hidden"`
	OverlayType     int     `json:"overlayType"`
	Art             string  `json:"art"`
	Animated        bool    `json:"animated"`
	Percent         float32 `json:"percent"`
	Goals           int     `json:"goals"`
	GoalsTotal      int     `json:"goalsTotal"`
	Unlocked        bool    `json:"unlocked"`
	NewUnlock       bool    `json:"newUnlock"`
}

type BadgePercentUnlocked struct {
	BadgeId string  `json:"badgeId"`
	Percent float32 `json:"percent"`
}

type TimeTrialRecord struct {
	MapId   int `json:"mapId"`
	Seconds int `json:"seconds"`
}

func initBadges() {
	// Use Yume 2kki server to update badge data
	if len(badges) > 0 && serverConfig.GameName == "2kki" {
		if _, ok := badges[serverConfig.GameName]; ok {
			// Badge records needed for determining badge game
			writeGameBadges()
			updatePlayerBadgeSlotCounts("")
		}
	}

	scheduler.Every(1).Tuesday().At("20:00").Do(func() {
		updateActiveBadgesAndConditions()
	})

	scheduler.Every(1).Friday().At("20:00").Do(func() {
		updateActiveBadgesAndConditions()
	})

	updateActiveBadgesAndConditions()
}

func updateActiveBadgesAndConditions() {
	firstBatchDate := time.Date(2022, time.April, 15, 20, 0, 0, 0, time.UTC)
	days := time.Now().UTC().Sub(firstBatchDate).Hours() / 24
	currentBatch := int(math.Floor(days/7)) + 1

	for game, gameBadges := range badges {
		for _, gameBadge := range gameBadges {
			if gameBadge.Batch > 0 {
				gameBadge.Dev = gameBadge.Batch > currentBatch
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
}

func getGlobalConditions() (globalConditions []*Condition) {
	if gameConditions, ok := conditions[serverConfig.GameName]; ok {
		for _, condition := range gameConditions {
			if condition.Map == 0 {
				globalConditions = append(globalConditions, condition)
			}
		}
	}
	return globalConditions
}

func getRoomConditions(roomId int) (roomConditions []*Condition) {
	if gameConditions, ok := conditions[serverConfig.GameName]; ok {
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
	if !c.sClient.account {
		return
	}

	for _, condition := range globalConditions {
		c.checkCondition(condition, 0, nil, trigger, value)
	}

	for _, condition := range c.room.conditions {
		c.checkCondition(condition, c.room.id, c.room.minigameConfigs, trigger, value)
	}
}

func (c *RoomClient) checkCondition(condition *Condition, roomId int, minigameConfigs []*MinigameConfig, trigger string, value string) {
	if condition.Disabled && c.sClient.rank < 2 {
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
		if (condition.SwitchId > 0 || len(condition.SwitchIds) > 0) && !condition.VarTrigger {
			switchId := condition.SwitchId
			if len(condition.SwitchIds) > 0 {
				switchId = condition.SwitchIds[0]
			}
			var switchSyncType int
			if trigger == "" {
				switchSyncType = 2
				if condition.SwitchDelay {
					switchSyncType = 1
				}
			}
			c.send <- buildMsg("ss", switchId, switchSyncType)
		} else if condition.VarId > 0 || len(condition.VarIds) > 0 {
			varId := condition.VarId
			if len(condition.VarIds) > 0 {
				varId = condition.VarIds[0]
			}

			if len(minigameConfigs) > 0 {
				var skipVarSync bool
				for _, minigame := range minigameConfigs {
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
			c.send <- buildMsg("sv", varId, varSyncType)
		} else if c.checkConditionCoords(condition) {
			timeTrial := condition.TimeTrial && serverConfig.GameName == "2kki"
			if !timeTrial {
				success, err := tryWritePlayerTag(c.sClient.uuid, condition.ConditionId)
				if err != nil {
					writeErrLog(c.sClient.uuid, c.mapId, err.Error())
				}
				if success {
					c.send <- buildMsg("b")
				}
			} else {
				c.send <- buildMsg("ss", "1430", "0")
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
					c.send <- buildMsg("sp", value)
				} else {
					valueInt, err := strconv.Atoi(value)
					if err != nil {
						writeErrLog(c.sClient.ip, strconv.Itoa(roomId), err.Error())
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

					c.send <- buildMsg("sev", value, eventTriggerType)
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

			playerBadge := &PlayerBadge{BadgeId: badgeId, Game: game, Group: gameBadge.Group, Bp: gameBadge.Bp, MapId: gameBadge.Map, MapX: gameBadge.MapX, MapY: gameBadge.MapY, Secret: gameBadge.Secret, SecretCondition: gameBadge.SecretCondition, OverlayType: gameBadge.OverlayType, Art: gameBadge.Art, Animated: gameBadge.Animated, Hidden: gameBadge.Hidden || gameBadge.Dev}
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
									playerBadge.Goals++
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

	var unlockPercentages []*BadgePercentUnlocked

	if !simple {
		unlockPercentages, err = getBadgeUnlockPercentages()
		if err != nil {
			return playerBadges, err
		}
	}

	var unlockedBadge bool

	for _, badge := range playerBadges {
		if !simple {
			for _, badgePercentUnlocked := range unlockPercentages {
				if badge.BadgeId == badgePercentUnlocked.BadgeId {
					badge.Percent = badgePercentUnlocked.Percent
					break
				}
			}
		}

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
				condition := &Condition{}

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
					} else if len(condition.VarIds) > 0 {
						if len(condition.VarOps) < len(condition.VarIds) {
							for v := range condition.VarIds {
								if v >= len(condition.VarOps) {
									condition.VarOps = append(condition.VarOps, "=")
								}
							}
						}
					}
					conditionConfig[gameId][conditionId] = condition
				}
			}
		}
	}

	conditions = conditionConfig
}

func setBadges() {
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
				badge := &Badge{}

				data, err := os.ReadFile(configPath + badgeConfigFile.Name())
				if err != nil {
					continue
				}

				err = json.Unmarshal(data, &badge)
				if err == nil {
					badgeId := badgeConfigFile.Name()[:len(badgeConfigFile.Name())-5]
					badgeConfig[gameId][badgeId] = badge
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
