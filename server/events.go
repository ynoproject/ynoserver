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
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type EventPeriod struct {
	Id            int       `json:"-"`
	PeriodOrdinal int       `json:"periodOrdinal"`
	EndDate       time.Time `json:"endDate"`
	EnableVms     bool      `json:"enableVms"`
}

type EventExp struct {
	WeekExp   int `json:"weekExp"`
	PeriodExp int `json:"periodExp"`
	TotalExp  int `json:"totalExp"`
}

type EventLocation struct {
	Id       int       `json:"id"`
	Type     int       `json:"type"`
	Game     string    `json:"game"`
	Title    string    `json:"title"`
	TitleJP  string    `json:"titleJP"`
	Depth    int       `json:"depth"`
	MinDepth int       `json:"minDepth,omitempty"`
	Exp      int       `json:"exp"`
	EndDate  time.Time `json:"endDate"`
	Complete bool      `json:"complete"`
}

type EventVm struct {
	Id       int       `json:"id"`
	Game     string    `json:"game"`
	Exp      int       `json:"exp"`
	EndDate  time.Time `json:"endDate"`
	Complete bool      `json:"complete"`
}

type EventsData struct {
	Locations []*EventLocation `json:"locations"`
	Vms       []*EventVm       `json:"vms"`
}

type EventLocationData struct {
	Title    string   `json:"title"`
	TitleJP  string   `json:"titleJP,omitempty"`
	Depth    int      `json:"depth"`
	MinDepth int      `json:"minDepth"`
	MapIds   []string `json:"mapIds"`
}

const (
	dailyEventLocationMinDepth = 2
	dailyEventLocationMaxDepth = 3
	dailyEventLocationExp      = 1

	dailyEventLocation2MinDepth = 4
	dailyEventLocation2MaxDepth = 6
	dailyEventLocation2Exp      = 3

	weeklyEventLocationMinDepth = 7
	weeklyEventLocationMaxDepth = 10
	weeklyEventLocationExp      = 10

	weekendEventLocationMinDepth = 6
	weekendEventLocationMaxDepth = 9
	weekendEventLocationExp      = 5

	eventLocationCountDailyThreshold   = 8
	eventLocationCountWeeklyThreshold  = 3
	eventLocationCountWeekendThreshold = 5

	freeEventLocationMinDepth = 2

	eventVmExp = 4

	weeklyExpCap = 50

	gameEventShareFactor = 0.25
)

const (
	daily2kkiEventLocationMinDepth = 3
	daily2kkiEventLocationMaxDepth = 5

	daily2kkiEventLocation2MinDepth = 5
	daily2kkiEventLocation2MaxDepth = 9

	weekly2kkiEventLocationMinDepth = 11
	weekly2kkiEventLocationMaxDepth = -1

	weekend2kkiEventLocationMinDepth = 9
	weekend2kkiEventLocationMaxDepth = 14
)

var (
	currentEventPeriodId     = -1
	currentGameEventPeriodId = -1
	currentEventVmMapId      int
	currentEventVmEventId    int
	eventsCount              int

	gameCurrentEventPeriods       map[string]*EventPeriod
	gameDailyEventLocationPools   map[string][]*EventLocationData
	gameDailyEventLocation2Pools  map[string][]*EventLocationData
	gameWeeklyEventLocationPools  map[string][]*EventLocationData
	gameWeekendEventLocationPools map[string][]*EventLocationData
	freeEventLocationPool         []*EventLocationData
	eventVms                      map[int][]int
)

func initEvents() {
	err := setCurrentEventPeriodId()
	if err != nil {
		return
	}

	err = setCurrentGameEventPeriodId()
	if err != nil {
		return
	}

	if currentGameEventPeriodId == 0 {
		return
	}

	if isHostServer {
		gameCurrentEventPeriods, err = getGameCurrentEventPeriodsData()
		if err != nil {
			return
		}
	}

	setGameEventLocationPools()

	if !isHostServer {
		return
	}

	db.QueryRow("SELECT COUNT(*) FROM eventLocations el").Scan(&eventsCount)

	scheduler.Every(1).Day().At("00:00").Do(func() {
		err := setCurrentEventPeriodId()
		if err != nil {
			return
		}

		err = setCurrentGameEventPeriodId()
		if err != nil {
			return
		}

		gameCurrentEventPeriods, err = getGameCurrentEventPeriodsData()
		if err != nil {
			return
		}

		addDailyEventLocation(false)
		addDailyEventLocation(true)
		eventsCount += 2

		switch time.Now().UTC().Weekday() {
		case time.Sunday:
			addWeeklyEventLocation()
			addEventVm()
			eventsCount += 2
		case time.Tuesday:
			addEventVm()
			eventsCount++
		case time.Friday:
			addWeekendEventLocation()
			addEventVm()
			eventsCount += 2
		}

		sendEventsUpdate()
	})

	scheduler.Every(5).Minutes().Do(func() {
		var newEventLocationsCount int
		db.QueryRow("SELECT COUNT(*) FROM eventLocations").Scan(&newEventLocationsCount)
		if newEventLocationsCount != eventsCount {
			eventsCount = newEventLocationsCount
			sendEventsUpdate()
		}
	})

	var count int

	// daily easy expedition
	db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE el.type = 0 AND ep.id = ? AND el.startDate = UTC_DATE() AND el.exp = 1", currentEventPeriodId).Scan(&count)
	if count == 0 {
		addDailyEventLocation(false)
	}

	// daily deeper expedition
	db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE el.type = 0 AND ep.id = ? AND el.startDate = UTC_DATE() AND el.exp = 3", currentEventPeriodId).Scan(&count)
	if count == 0 {
		addDailyEventLocation(true)
	}

	weekday := time.Now().UTC().Weekday()

	// weekly expedition
	db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE el.type = 1 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", currentEventPeriodId, int(weekday)).Scan(&count)
	if count == 0 {
		addWeeklyEventLocation()
	}

	var lastVmWeekday time.Weekday

	switch weekday {
	case time.Sunday, time.Monday:
		lastVmWeekday = time.Sunday
	case time.Tuesday, time.Wednesday, time.Thursday:
		lastVmWeekday = time.Tuesday
	case time.Friday, time.Saturday:
		// weekend expedition
		db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN gameEventPeriods gep ON gep.id = el.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE el.type = 2 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", currentEventPeriodId, int(weekday-time.Friday)).Scan(&count)
		if count == 0 {
			addWeekendEventLocation()
		}

		lastVmWeekday = time.Friday
	}

	// vending machine expedition
	db.QueryRow("SELECT ev.mapId, ev.eventId FROM eventVms ev JOIN gameEventPeriods gep ON gep.id = ev.gamePeriodId JOIN eventPeriods ep ON ep.id = gep.periodId WHERE ep.id = ? AND ev.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", currentEventPeriodId, int(weekday-lastVmWeekday)).Scan(&currentEventVmMapId, &currentEventVmEventId)
	if currentEventVmMapId == 0 && currentEventVmEventId == 0 {
		addEventVm()
	}
}

func sendEventsUpdate() {
	clients.Range(func(_, v any) bool {
		client := v.(*SessionClient)
		if client.account {
			client.handleE()
		}

		return true
	})
}

func addDailyEventLocation(deeper bool) {
	var pools map[string][]*EventLocationData
	if !deeper {
		pools = gameDailyEventLocationPools
	} else {
		pools = gameDailyEventLocation2Pools
	}

	gameId, err := getRandomGameForEventLocation(pools, eventLocationCountDailyThreshold)
	if err != nil {
		handleInternalEventError(0, err)
		return
	}

	if gameId == "2kki" {
		if !deeper {
			add2kkiEventLocation(0, daily2kkiEventLocationMinDepth, daily2kkiEventLocationMaxDepth, dailyEventLocationExp)
		} else {
			add2kkiEventLocation(0, daily2kkiEventLocation2MinDepth, daily2kkiEventLocation2MaxDepth, dailyEventLocation2Exp)
		}
	} else {
		if !deeper {
			addEventLocation(gameId, 0, dailyEventLocationExp, pools)
		} else {
			addEventLocation(gameId, 0, dailyEventLocation2Exp, pools)
		}
	}
}

func addWeeklyEventLocation() {
	gameId, err := getRandomGameForEventLocation(gameWeeklyEventLocationPools, eventLocationCountWeeklyThreshold)
	if err != nil {
		handleInternalEventError(1, err)
		return
	}

	if gameId == "2kki" {
		add2kkiEventLocation(1, weekly2kkiEventLocationMinDepth, weekly2kkiEventLocationMaxDepth, weeklyEventLocationExp)
	} else {
		addEventLocation(gameId, 1, weeklyEventLocationExp, gameWeeklyEventLocationPools)
	}
}

func addWeekendEventLocation() {
	gameId, err := getRandomGameForEventLocation(gameWeekendEventLocationPools, eventLocationCountWeekendThreshold)
	if err != nil {
		handleInternalEventError(2, err)
		return
	}

	if gameId == "2kki" {
		add2kkiEventLocation(2, weekend2kkiEventLocationMinDepth, weekend2kkiEventLocationMaxDepth, weekendEventLocationExp)
	} else {
		addEventLocation(gameId, 2, weekendEventLocationExp, gameWeekendEventLocationPools)
	}
}

func addEventLocation(gameId string, eventType int, exp int, pools map[string][]*EventLocationData) {
	addPlayerEventLocation(gameId, eventType, exp, pools[gameId], "")
}

// eventType: 0 - daily, 1 - weekly, 2 - weekend, 3 - manual
func addPlayerEventLocation(gameId string, eventType int, exp int, pool []*EventLocationData, playerUuid string) {
	rand.Seed(time.Now().Unix())
	eventLocation := pool[rand.Intn(len(pool))]

	var gameEventPeriodId int
	if gameId == serverConfig.GameName {
		gameEventPeriodId = currentGameEventPeriodId
	} else {
		gameEventPeriodId = gameCurrentEventPeriods[gameId].Id
	}

	var err error
	if playerUuid == "" {
		err = writeEventLocationData(gameEventPeriodId, eventType, eventLocation.Title, eventLocation.TitleJP, eventLocation.Depth, eventLocation.MinDepth, exp, eventLocation.MapIds)
	} else {
		err = writePlayerEventLocationData(gameEventPeriodId, playerUuid, eventLocation.Title, eventLocation.TitleJP, eventLocation.Depth, eventLocation.MinDepth, eventLocation.MapIds)
	}
	if err != nil {
		handleInternalEventError(eventType, err)
	}
}

func add2kkiEventLocation(eventType int, minDepth int, maxDepth int, exp int) {
	var gameEventPeriodId int
	if serverConfig.GameName == "2kki" {
		gameEventPeriodId = currentGameEventPeriodId
	} else {
		gameEventPeriodId = gameCurrentEventPeriods["2kki"].Id
	}
	addPlayer2kkiEventLocation(gameEventPeriodId, eventType, minDepth, maxDepth, exp, "")
}

// eventType: 0 - daily, 1 - weekly, 2 - weekend, 3 - manual
func addPlayer2kkiEventLocation(gameEventPeriodId int, eventType int, minDepth int, maxDepth int, exp int, playerUuid string) {
	url := "https://2kki.app/getRandomLocations?ignoreSecret=1&minDepth=" + strconv.Itoa(minDepth)
	if maxDepth >= minDepth {
		url += "&maxDepth=" + strconv.Itoa(maxDepth)
	}

	resp, err := http.Get(url)
	if err != nil {
		handleInternalEventError(eventType, err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		handleInternalEventError(eventType, err)
		return
	}

	if strings.HasPrefix(string(body), "{\"error\"") {
		handleEventError(eventType, "Invalid event location data: "+string(body))
		return
	}

	var eventLocations []EventLocationData
	err = json.Unmarshal(body, &eventLocations)
	if err != nil {
		handleInternalEventError(eventType, err)
		return
	}

	for _, eventLocation := range eventLocations {
		adjustedDepth := (eventLocation.Depth / 3) * 2
		if eventLocation.Depth%3 == 2 {
			adjustedDepth++
		}
		if adjustedDepth > 10 {
			adjustedDepth = 10
		}

		var adjustedMinDepth int
		if eventLocation.MinDepth == eventLocation.Depth {
			adjustedMinDepth = adjustedDepth
		} else {
			adjustedMinDepth = (eventLocation.MinDepth / 3) * 2
			if eventLocation.MinDepth%3 == 2 {
				adjustedMinDepth++
			}
			if adjustedMinDepth > 10 {
				adjustedMinDepth = 10
			}
		}
		if playerUuid == "" {
			err = writeEventLocationData(gameEventPeriodId, eventType, eventLocation.Title, eventLocation.TitleJP, adjustedDepth, adjustedMinDepth, exp, eventLocation.MapIds)
		} else {
			err = writePlayerEventLocationData(gameEventPeriodId, playerUuid, eventLocation.Title, eventLocation.TitleJP, adjustedDepth, adjustedMinDepth, eventLocation.MapIds)
		}
		if err != nil {
			handleInternalEventError(eventType, err)
		}
	}
}

func addEventVm() {
	mapIds := make([]int, 0, len(eventVms))
	for k := range eventVms {
		mapIds = append(mapIds, k)
	}
	if len(mapIds) == 0 {
		return
	}

	rand.Seed(time.Now().Unix())
	mapId := mapIds[rand.Intn(len(mapIds))]
	eventId := eventVms[mapId][rand.Intn(len(eventVms[mapId]))]

	err := writeEventVmData(mapId, eventId, eventVmExp)
	if err == nil {
		currentEventVmMapId = mapId
		currentEventVmEventId = eventId
	} else {
		writeErrLog("SERVER", "VM", err.Error())
	}
}

func handleInternalEventError(eventType int, err error) {
	handleEventError(eventType, err.Error())
}

func handleEventError(eventType int, payload string) {
	writeErrLog("SERVER", strconv.Itoa(eventType), payload)
}

func setEventVms() {
	if !isHostServer {
		return
	}

	vmsDir, err := os.ReadDir("vms/")
	if err != nil {
		return
	}

	eventVms = make(map[int][]int)

	for _, vmFile := range vmsDir {
		mapIdInt, err := strconv.Atoi(vmFile.Name()[3:7])
		if err != nil {
			return
		}

		eventIdInt, err := strconv.Atoi(vmFile.Name()[10:14])
		if err != nil {
			return
		}

		eventVms[mapIdInt] = append(eventVms[mapIdInt], eventIdInt)
	}
}

func setGameEventLocationPools() {
	if isHostServer {
		gameDailyEventLocationPools = make(map[string][]*EventLocationData)
		gameDailyEventLocation2Pools = make(map[string][]*EventLocationData)
		gameWeeklyEventLocationPools = make(map[string][]*EventLocationData)
		gameWeekendEventLocationPools = make(map[string][]*EventLocationData)
	}

	gameEventLocations := make(map[string][]*EventLocationData)
	gameMaxDepths := make(map[string]int)

	configPath := "eventlocations/"

	var gameIds []string
	if isHostServer {
		for gameId := range gameCurrentEventPeriods {
			gameIds = append(gameIds, gameId)
		}
	} else {
		gameIds = append(gameIds, serverConfig.GameName)
	}

	for _, gameId := range gameIds {
		var eventLocations []*EventLocationData

		data, err := os.ReadFile(configPath + gameId + ".json")
		if err != nil {
			continue
		}

		err = json.Unmarshal(data, &eventLocations)
		if err != nil {
			continue
		}

		if len(eventLocations) > 0 {
			gameEventLocations[gameId] = eventLocations
			gameMaxDepths[gameId] = 0
		}

		for _, eventLocation := range eventLocations {
			if eventLocation.Depth > gameMaxDepths[gameId] {
				gameMaxDepths[gameId] = eventLocation.Depth
			}
		}
	}

	for gameId, eventLocations := range gameEventLocations {
		gameMaxDepth := math.Min(float64(gameMaxDepths[gameId]), 15)

		for _, eventLocation := range eventLocations {
			adjustedDepth := eventLocation.Depth
			adjustedMinDepth := eventLocation.MinDepth
			if gameMaxDepth > 10 {
				adjustedDepth = int(math.Floor(float64(adjustedDepth) / gameMaxDepth * 10))
				adjustedMinDepth = int(math.Floor(float64(adjustedMinDepth) / gameMaxDepth * 10))
			}
			eventLocation.Depth = adjustedDepth
			eventLocation.MinDepth = adjustedMinDepth

			if isHostServer {
				if adjustedDepth >= dailyEventLocationMinDepth && adjustedDepth <= dailyEventLocationMaxDepth {
					gameDailyEventLocationPools[gameId] = append(gameDailyEventLocationPools[gameId], eventLocation)
				}
				if adjustedDepth >= dailyEventLocation2MinDepth && adjustedDepth <= dailyEventLocation2MaxDepth {
					gameDailyEventLocation2Pools[gameId] = append(gameDailyEventLocation2Pools[gameId], eventLocation)
				}
				if adjustedDepth >= weeklyEventLocationMinDepth && adjustedDepth <= weeklyEventLocationMaxDepth {
					gameWeeklyEventLocationPools[gameId] = append(gameWeeklyEventLocationPools[gameId], eventLocation)
				}
				if adjustedDepth >= weekendEventLocationMinDepth && adjustedDepth <= weekendEventLocationMaxDepth {
					gameWeekendEventLocationPools[gameId] = append(gameWeekendEventLocationPools[gameId], eventLocation)
				}
			}
			if gameId == serverConfig.GameName && adjustedDepth >= freeEventLocationMinDepth {
				freeEventLocationPool = append(freeEventLocationPool, eventLocation)
			}
		}
	}
}
