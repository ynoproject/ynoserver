package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
)

type EventPeriod struct {
	PeriodOrdinal int       `json:"periodOrdinal"`
	EndDate       time.Time `json:"endDate"`
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
	Exp      int       `json:"exp"`
	EndDate  time.Time `json:"endDate"`
	Complete bool      `json:"complete"`
}

type EventLocationData struct {
	Title   string   `json:"title"`
	TitleJP string   `json:"titleJP"`
	Depth   int      `json:"depth"`
	MapIds  []string `json:"mapIds"`
}

func StartEvents() {
	if config.gameName == "2kki" {
		s := gocron.NewScheduler(time.UTC)

		periodId, err := readCurrentEventPeriodId()
		if err == nil {
			var count int

			result := db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 0 AND ep.id = ? AND el.startDate = UTC_DATE()", periodId)
			result.Scan(&count)

			if count < 2 {
				add2kkiEventLocations(0, 2-count)
			}

			weekday := time.Now().UTC().Weekday()

			result = db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 1 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday))
			result.Scan(&count)

			if count < 1 {
				add2kkiEventLocations(1, 1)
			}

			if weekday == time.Friday || weekday == time.Saturday {
				result = db.QueryRow("SELECT COUNT(el.id) FROM eventLocations el JOIN eventPeriods ep ON ep.id = el.periodId WHERE el.type = 2 AND ep.id = ? AND el.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday)-int(time.Friday))
				result.Scan(&count)

				if count < 1 {
					add2kkiEventLocations(2, 1)
				}
			}
		}

		s.Every(1).Day().At("00:00").Do(func() {
			add2kkiEventLocations(0, 2)
		})

		s.Every(1).Sunday().At("00:00").Do(func() {
			add2kkiEventLocations(1, 1)
		})

		s.Every(1).Friday().At("00:00").Do(func() {
			add2kkiEventLocations(2, 1)
		})

		s.StartAsync()
	}
}

func add2kkiEventLocations(eventType int, count int) {
	exp := 2
	if eventType == 1 {
		exp = 10
	} else if eventType == 2 {
		exp = 5
	}

	add2kkiEventLocationsWithExp(eventType, count, exp, "")
}

func add2kkiEventLocationsWithExp(eventType int, count int, exp int, playerUuid string) {
	periodId, err := readCurrentEventPeriodId()
	if err != nil {
		handleInternalEventError(eventType, err)
		return
	}

	url := "https://2kki.app/getRandomLocations?count=" + strconv.Itoa(count) + "&ignoreSecret=1"
	if eventType == 0 {
		url += "&minDepth=3&maxDepth=9"
	} else if eventType == 1 {
		url += "&minDepth=11"
	} else if eventType == 2 {
		url += "&minDepth=9&maxDepth=14"
	} else {
		url += "&minDepth=2"
	}

	resp, err := http.Get(url)
	if err != nil {
		handleInternalEventError(eventType, err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
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
			adjustedDepth += 1
		}
		if adjustedDepth > 10 {
			adjustedDepth = 10
		}
		if playerUuid == "" {
			err = writeEventLocationData(periodId, eventType, eventLocation.Title, eventLocation.TitleJP, adjustedDepth, exp, eventLocation.MapIds)
		} else {
			err = writePlayerEventLocationData(periodId, playerUuid, eventLocation.Title, eventLocation.TitleJP, adjustedDepth, eventLocation.MapIds)
		}
		if err != nil {
			handleInternalEventError(eventType, err)
		}
	}
}

func handleInternalEventError(eventType int, err error) {
	handleEventError(eventType, err.Error())
}

func handleEventError(eventType int, payload string) {
	writeErrLog("SERVER", strconv.Itoa(eventType), payload)
}
