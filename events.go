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
			sendEventsUpdate()
		})

		s.Every(1).Sunday().At("00:00").Do(func() {
			add2kkiEventLocation(1, weeklyEventLocationMinDepth, weeklyEventLocationMaxDepth, weeklyEventLocationExp)
			add2kkiEventVm()
			eventsCount += 2
			sendEventsUpdate()
		})

		s.Every(1).Tuesday().At("00:00").Do(func() {
			add2kkiEventVm()
			eventsCount++
			sendEventsUpdate()
		})

		s.Every(1).Friday().At("00:00").Do(func() {
			add2kkiEventLocation(2, weekendEventLocationMinDepth, weekendEventLocationMaxDepth, weekendEventLocationExp)
			add2kkiEventVm()
			eventsCount += 2
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

		periodId, err := readCurrentEventPeriodId()
		if err == nil {
			var count int

			db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 0 AND ep.id = ? AND el.startDate = UTC_DATE() AND el.exp = 1", periodId).Scan(&count)

			if count < 1 {
				add2kkiEventLocation(0, dailyEventLocationMinDepth, dailyEventLocationMaxDepth, dailyEventLocationExp)
			}

			db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 0 AND ep.id = ? AND el.startDate = UTC_DATE() AND el.exp = 3", periodId).Scan(&count)

			if count < 1 {
				add2kkiEventLocation(0, dailyEventLocation2MinDepth, dailyEventLocation2MaxDepth, dailyEventLocation2Exp)
			}

			weekday := time.Now().UTC().Weekday()

			db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 1 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday)).Scan(&count)

			if count < 1 {
				add2kkiEventLocation(1, weeklyEventLocationMinDepth, weeklyEventLocationMaxDepth, weeklyEventLocationExp)
			}

			var lastVmWeekday time.Weekday

			switch weekday {
			case time.Sunday:
				fallthrough
			case time.Monday:
				lastVmWeekday = time.Sunday
			case time.Tuesday:
				fallthrough
			case time.Wednesday:
				fallthrough
			case time.Thursday:
				lastVmWeekday = time.Tuesday
			case time.Friday:
				fallthrough
			case time.Saturday:
				db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 2 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday)-int(time.Friday)).Scan(&count)

				if count < 1 {
					add2kkiEventLocation(2, weekendEventLocationMinDepth, weekendEventLocationMaxDepth, weekendEventLocationExp)
				}

				lastVmWeekday = time.Friday
			}

			err = db.QueryRow("SELECT ev.mapId, ev.eventId FROM eventVms ev JOIN eventPeriods ep ON ep.id = ev.periodId WHERE ep.id = ? AND ev.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday)-int(lastVmWeekday)).Scan(&currentEventVmMapId, &currentEventVmEventId)

			if err == sql.ErrNoRows {
				add2kkiEventVm()
			}
		}
	}
}

func sendEventsUpdate() {
	var emptyMsg []string
	for _, sessionClient := range sessionClients {
		if sessionClient.account {
			session.handleE(emptyMsg, sessionClient)
		}
	}
}

func add2kkiEventLocation(eventType int, minDepth int, maxDepth int, exp int) {
	addPlayer2kkiEventLocation(eventType, minDepth, maxDepth, exp, "")
}

func addPlayer2kkiEventLocation(eventType int, minDepth int, maxDepth int, exp int, playerUuid string) {
	periodId, err := readCurrentEventPeriodId()
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
	periodId, err := readCurrentEventPeriodId()
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
	eventVms = make(map[int][]int)

	if config.gameName != "2kki" {
		return
	}

	vmsDir, err := os.ReadDir("vms/")
	if err != nil {
		return
	}

	for _, vmFile := range vmsDir {
		mapId := vmFile.Name()[3:7]
		eventId := vmFile.Name()[10:14]

		mapIdInt, err := strconv.Atoi(mapId)
		if err != nil {
			return
		}

		eventIdInt, err := strconv.Atoi(eventId)
		if err != nil {
			return
		}

		eventVms[mapIdInt] = append(eventVms[mapIdInt], eventIdInt)
	}
}
