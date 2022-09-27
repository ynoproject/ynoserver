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

package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
)

var (
	eventVms map[int][]int
)

type EventPeriod struct {
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
	TitleJP  string   `json:"titleJP"`
	Depth    int      `json:"depth"`
	MinDepth int      `json:"minDepth"`
	MapIds   []string `json:"mapIds"`
}

const (
	dailyEventLocationMinDepth = 3
	dailyEventLocationMaxDepth = 5
	dailyEventLocationExp      = 1

	dailyEventLocation2MinDepth = 5
	dailyEventLocation2MaxDepth = 9
	dailyEventLocation2Exp      = 3

	weeklyEventLocationMinDepth = 11
	weeklyEventLocationMaxDepth = -1
	weeklyEventLocationExp      = 10

	weekendEventLocationMinDepth = 9
	weekendEventLocationMaxDepth = 14
	weekendEventLocationExp      = 5

	eventVmExp = 4

	weeklyExpCap = 50
)

var (
	currentEventPeriodId  int = -1
	currentEventVmMapId   int
	currentEventVmEventId int
	eventsCount           int
)

func initEvents() {
	if config.gameName == "2kki" {
		s := gocron.NewScheduler(time.UTC)

		db.QueryRow("SELECT COUNT(*) FROM eventLocations").Scan(&eventsCount)

		s.Every(1).Day().At("00:00").Do(func() {
			add2kkiEventLocation(0, dailyEventLocationMinDepth, dailyEventLocationMaxDepth, dailyEventLocationExp)
			add2kkiEventLocation(0, dailyEventLocation2MinDepth, dailyEventLocation2MaxDepth, dailyEventLocation2Exp)
			eventsCount += 2

			switch time.Now().UTC().Weekday() {
			case time.Sunday:
				add2kkiEventLocation(1, weeklyEventLocationMinDepth, weeklyEventLocationMaxDepth, weeklyEventLocationExp)
				add2kkiEventVm()
				eventsCount += 2
			case time.Tuesday:
				add2kkiEventVm()
				eventsCount++
			case time.Friday:
				add2kkiEventLocation(2, weekendEventLocationMinDepth, weekendEventLocationMaxDepth, weekendEventLocationExp)
				add2kkiEventVm()
				eventsCount += 2
			}

			sendEventsUpdate()
		})

		s.Every(5).Minutes().Do(func() {
			var newEventLocationsCount int
			db.QueryRow("SELECT COUNT(*) FROM eventLocations").Scan(&newEventLocationsCount)
			if newEventLocationsCount != eventsCount {
				eventsCount = newEventLocationsCount
				sendEventsUpdate()
			}
		})

		s.StartAsync()

		periodId, err := getCurrentEventPeriodId()
		if err == nil {
			var count int

			// daily easy expedition
			db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 0 AND ep.id = ? AND el.startDate = UTC_DATE() AND el.exp = 1", periodId).Scan(&count)
			if count < 1 {
				add2kkiEventLocation(0, dailyEventLocationMinDepth, dailyEventLocationMaxDepth, dailyEventLocationExp)
			}

			// daily hard expedition
			db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 0 AND ep.id = ? AND el.startDate = UTC_DATE() AND el.exp = 3", periodId).Scan(&count)
			if count < 1 {
				add2kkiEventLocation(0, dailyEventLocation2MinDepth, dailyEventLocation2MaxDepth, dailyEventLocation2Exp)
			}

			weekday := time.Now().UTC().Weekday()

			// weekly expedition
			db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 1 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday)).Scan(&count)
			if count < 1 {
				add2kkiEventLocation(1, weeklyEventLocationMinDepth, weeklyEventLocationMaxDepth, weeklyEventLocationExp)
			}

			var lastVmWeekday time.Weekday

			switch weekday {
			case time.Sunday, time.Monday:
				lastVmWeekday = time.Sunday
			case time.Tuesday, time.Wednesday, time.Thursday:
				lastVmWeekday = time.Tuesday
			case time.Friday, time.Saturday:
				// weekend expedition
				db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 2 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday-time.Friday)).Scan(&count)
				if count < 1 {
					add2kkiEventLocation(2, weekendEventLocationMinDepth, weekendEventLocationMaxDepth, weekendEventLocationExp)
				}

				lastVmWeekday = time.Friday
			}

			// vending machine expedition
			db.QueryRow("SELECT ev.mapId, ev.eventId FROM eventVms ev JOIN eventPeriods ep ON ep.id = ev.periodId WHERE ep.id = ? AND ev.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday-lastVmWeekday)).Scan(&currentEventVmMapId, &currentEventVmEventId)
			if err == sql.ErrNoRows {
				add2kkiEventVm()
			}
		}
	}
}

func sendEventsUpdate() {
	sessionClients.Range(func(_, v any) bool {
		sessionClient := v.(*SessionClient)

		if sessionClient.account {
			session.handleE(sessionClient)
		}

		return true
	})
}

func add2kkiEventLocation(eventType int, minDepth int, maxDepth int, exp int) {
	addPlayer2kkiEventLocation(eventType, minDepth, maxDepth, exp, "")
}

// eventType: 0 - daily, 1 - weekly, 2 - weekend, 3 - manual
func addPlayer2kkiEventLocation(eventType int, minDepth int, maxDepth int, exp int, playerUuid string) {
	periodId, err := getCurrentEventPeriodId()
	if err != nil {
		handleInternalEventError(eventType, err)
		return
	}

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
			err = writeEventLocationData(periodId, eventType, eventLocation.Title, eventLocation.TitleJP, adjustedDepth, adjustedMinDepth, exp, eventLocation.MapIds)
		} else {
			err = writePlayerEventLocationData(periodId, playerUuid, eventLocation.Title, eventLocation.TitleJP, adjustedDepth, adjustedMinDepth, eventLocation.MapIds)
		}
		if err != nil {
			handleInternalEventError(eventType, err)
		}
	}
}

func add2kkiEventVm() {
	periodId, err := getCurrentEventPeriodId()
	if err != nil {
		writeErrLog("SERVER", "VM", err.Error())
		return
	}

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

	err = writeEventVmData(periodId, mapId, eventId, eventVmExp)
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
	if config.gameName != "2kki" {
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
