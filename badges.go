package main

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
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
				if c.SwitchValue == value {
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
		valid := false
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
				valid := false
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
	Group           string   `json:"group"`
	Order           int      `json:"order"`
	MapOrder        int      `json:"mapOrder"`
	Bp              int      `json:"bp"`
	ReqType         string   `json:"reqType"`
	ReqInt          int      `json:"reqInt"`
	ReqString       string   `json:"reqString"`
	ReqStrings      []string `json:"reqStrings"`
	ReqCount        int      `json:"reqCount"`
	Map             int      `json:"map"`
	MapX            int      `json:"mapX"`
	MapY            int      `json:"mapY"`
	Secret          bool     `json:"secret"`
	SecretMap       bool     `json:"secretMap"`
	SecretCondition bool     `json:"secretCondition"`
	Hidden          bool     `json:"hidden"`
	Parent          string   `json:"parent"`
	Overlay         bool     `json:"overlay"`
	MaskOverlay     bool     `json:"maskOverlay"`
	LocOverlay      bool     `json:"locOverlay"`
	Art             string   `json:"art"`
	Animated        bool     `json:"animated"`
	Batch           int      `json:"batch"`
	Dev             bool     `json:"dev"`
}

type SimplePlayerBadge struct {
	BadgeId     string `json:"badgeId"`
	Game        string `json:"game"`
	Group       string `json:"group"`
	Hidden      bool   `json:"hidden"`
	Overlay     bool   `json:"overlay"`
	MaskOverlay bool   `json:"maskOverlay"`
	LocOverlay  bool   `json:"locOverlay"`
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
	Overlay         bool    `json:"overlay"`
	MaskOverlay     bool    `json:"maskOverlay"`
	LocOverlay      bool    `json:"locOverlay"`
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
	s := gocron.NewScheduler(time.UTC)

	s.Every(1).Tuesday().At("20:00").Do(func() {
		updateActiveBadgesAndConditions()
	})

	s.Every(1).Friday().At("20:00").Do(func() {
		updateActiveBadgesAndConditions()
	})

	updateActiveBadgesAndConditions()

	s.StartAsync()
}

func updateActiveBadgesAndConditions() {
	firstBatchDate := time.Date(2022, time.May, 6, 20, 0, 0, 0, time.UTC)
	days := time.Now().UTC().Sub(firstBatchDate).Hours() / 24
	currentBatch := int(math.Floor(days/7)*2) + 1
	if math.Mod(days, 7) >= 4 {
		currentBatch++
	}

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
				}
			}
		}
	}
}

func getHubConditions(roomName string) (hubConditions []*Condition) {
	if gameConditions, ok := conditions[config.gameName]; ok {
		mapId, _ := strconv.Atoi(roomName)
		for _, condition := range gameConditions {
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
		if c.Disabled && client.rank < 2 {
			continue
		}

		valueMatched := trigger == ""
		if c.Trigger == trigger && !valueMatched {
			if len(c.Values) == 0 {
				valueMatched = value == c.Value
			} else {
				for _, val := range c.Values {
					if value == val {
						valueMatched = true
						break
					}
				}
			}
		}

		if c.Trigger == trigger && valueMatched {
			if (c.SwitchId > 0 || len(c.SwitchIds) > 0) && !c.VarTrigger {
				switchId := c.SwitchId
				if len(c.SwitchIds) > 0 {
					switchId = c.SwitchIds[0]
				}
				switchSyncType := 0
				if trigger == "" {
					switchSyncType = 2
					if c.SwitchDelay {
						switchSyncType = 1
					}
				}
				client.send <- []byte("ss" + paramDelimStr + strconv.Itoa(switchId) + paramDelimStr + strconv.Itoa(switchSyncType))
			} else if c.VarId > 0 || len(c.VarIds) > 0 {
				varId := c.VarId
				if len(c.VarIds) > 0 {
					varId = c.VarIds[0]
				}

				if len(h.minigameConfigs) > 0 {
					skipVarSync := false
					for _, minigame := range h.minigameConfigs {
						if minigame.VarId == varId {
							skipVarSync = true
							break
						}
					}
					if skipVarSync {
						continue
					}
				}

				varSyncType := 0
				if trigger == "" {
					varSyncType = 2
					if c.VarDelay {
						varSyncType = 1
					}
				}
				client.send <- []byte("sv" + paramDelimStr + strconv.Itoa(varId) + paramDelimStr + strconv.Itoa(varSyncType))
			} else if checkConditionCoords(c, client) {
				timeTrial := c.TimeTrial && config.gameName == "2kki"
				if !timeTrial {
					success, err := tryWritePlayerTag(client.uuid, c.ConditionId)
					if err != nil {
						writeErrLog(client.ip, h.roomName, err.Error())
					}
					if success {
						client.send <- []byte("b")
					}
				} else {
					client.send <- []byte("ss" + paramDelimStr + "1430" + paramDelimStr + "0")
				}
			}
		} else if trigger == "" && (c.Trigger == "event" || c.Trigger == "eventAction") {
			var values []string
			if len(c.Values) == 0 {
				values = append(values, c.Value)
			} else {
				values = c.Values
			}
			for _, value := range values {
				_, err := strconv.Atoi(value)
				if err != nil {
					writeErrLog(client.ip, h.roomName, err.Error())
					continue
				}
				eventTriggerType := 0
				if c.Trigger == "eventAction" {
					eventTriggerType = 1
				}
				client.send <- []byte("sev" + paramDelimStr + value + paramDelimStr + strconv.Itoa(eventTriggerType))
			}
		}
	}
}

func checkConditionCoords(condition *Condition, client *Client) bool {
	return ((condition.MapX1 == 0 && condition.MapX2 == 0) || (condition.MapX1 <= client.x && condition.MapX2 >= client.x)) && ((condition.MapY1 == 0 && condition.MapY2 == 0) || (condition.MapY1 <= client.y && condition.MapY2 >= client.y))
}

func readPlayerBadgeData(playerUuid string, playerRank int, playerTags []string, loggedIn bool, simple bool) (playerBadges []*PlayerBadge, err error) {
	playerExp := 0
	playerEventLocationCount := 0
	playerEventLocationCompletion := 0
	playerBadgeCount := 0
	var timeTrialRecords []*TimeTrialRecord

	if loggedIn {
		playerExp, err = readPlayerTotalEventExp(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		playerEventLocationCount, err = readPlayerEventLocationCount(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		playerEventLocationCompletion, err = readPlayerEventLocationCompletion(playerUuid)
		if err != nil {
			return playerBadges, err
		}
		timeTrialRecords, err = readPlayerTimeTrialRecords(playerUuid)
		if err != nil {
			return playerBadges, err
		}
	}

	playerBadgesMap := make(map[string]*PlayerBadge)
	var badgeCountPlayerBadges []*PlayerBadge

	var playerUnlockedBadgeIds []string

	if loggedIn {
		playerUnlockedBadgeIds, err = readPlayerUnlockedBadgeIds(playerUuid)
		if err != nil {
			return playerBadges, err
		}
	}

	for game, gameBadges := range badges {
		for badgeId, gameBadge := range gameBadges {
			if gameBadge.Dev && playerRank < 2 {
				continue
			}

			playerBadge := &PlayerBadge{BadgeId: badgeId, Game: game, Group: gameBadge.Group, Bp: gameBadge.Bp, MapId: gameBadge.Map, MapX: gameBadge.MapX, MapY: gameBadge.MapY, Secret: gameBadge.Secret, SecretCondition: gameBadge.SecretCondition, Overlay: gameBadge.Overlay, MaskOverlay: gameBadge.MaskOverlay, LocOverlay: gameBadge.LocOverlay, Art: gameBadge.Art, Animated: gameBadge.Animated, Hidden: gameBadge.Hidden || gameBadge.Dev}
			if gameBadge.SecretMap {
				playerBadge.MapId = 0
			}

			if loggedIn {
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
				case "exp":
					playerBadge.Goals = playerExp
					playerBadge.GoalsTotal = gameBadge.ReqInt
				case "expCount":
					playerBadge.Goals = playerEventLocationCount
					playerBadge.GoalsTotal = gameBadge.ReqInt
				case "expCompletion":
					playerBadge.Goals = playerEventLocationCompletion
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
				if !gameBadge.Hidden {
					playerBadgeCount++
				}
			} else if !simple && gameBadge.Hidden && playerRank < 2 {
				continue
			}

			playerBadgesMap[badgeId] = playerBadge
		}
	}

	for badgeId, playerBadge := range playerBadgesMap {
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

	if !simple {
		sort.Slice(playerBadges, func(a, b int) bool {
			playerBadgeA := playerBadges[a]
			playerBadgeB := playerBadges[b]

			if playerBadgeA.Game != playerBadgeB.Game {
				return strings.Compare(playerBadgeA.Game, playerBadgeB.Game) == -1
			}

			if playerBadgeA.Group != playerBadgeB.Group {
				return strings.Compare(playerBadgeA.Group, playerBadgeB.Group) == -1
			}

			gameBadgeA := badges[playerBadgeA.Game][playerBadgeA.BadgeId]
			gameBadgeB := badges[playerBadgeB.Game][playerBadgeB.BadgeId]

			if gameBadgeA.Order != gameBadgeB.Order {
				return gameBadgeA.Order < gameBadgeB.Order
			} else if gameBadgeA.Map != gameBadgeB.Map {
				sortMapA := gameBadgeA.Map
				sortMapB := gameBadgeB.Map

				if sortMapA == 0 {
					sortMapA = 9999
				} else if sortMapB == 0 {
					sortMapB = 9999
				}

				return sortMapA < sortMapB
			}

			return gameBadgeA.MapOrder < gameBadgeB.MapOrder
		})
	}

	var unlockPercentages []*BadgePercentUnlocked

	if !simple {
		unlockPercentages, err = readBadgeUnlockPercentages()
		if err != nil {
			return playerBadges, err
		}
	}

	newUnlockedBadgeCount := 0

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
				newUnlockedBadgeCount++
			}
		}
	}

	if newUnlockedBadgeCount > 0 {
		playerBadgeCount += newUnlockedBadgeCount
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
				newUnlockedBadgeCount++
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

func readSimplePlayerBadgeData(playerUuid string, playerRank int, playerTags []string, loggedIn bool) (playerBadges []*SimplePlayerBadge, err error) {
	badgeData, err := readPlayerBadgeData(playerUuid, playerRank, playerTags, loggedIn, true)
	if err != nil {
		return playerBadges, err
	}

	for _, badge := range badgeData {
		simpleBadge := &SimplePlayerBadge{BadgeId: badge.BadgeId, Game: badge.Game, Group: badge.Group, Hidden: badge.Hidden, Overlay: badge.Overlay, MaskOverlay: badge.MaskOverlay, LocOverlay: badge.LocOverlay, Animated: badge.Animated, Unlocked: badge.Unlocked, NewUnlock: badge.NewUnlock}
		playerBadges = append(playerBadges, simpleBadge)
	}

	return playerBadges, nil
}

func readPlayerNewUnlockedBadgeIds(playerUuid string, playerRank int, playerTags []string) (badgeIds []string, err error) {
	badgeData, err := readPlayerBadgeData(playerUuid, playerRank, playerTags, true, true)
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
