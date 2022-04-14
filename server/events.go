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

func StartEvents() {
	if config.gameName == "2kki" {
		s := gocron.NewScheduler(time.UTC)

		periodId, err := readCurrentEventPeriodId()
		if err == nil {
			var count int

			result := db.QueryRow("SELECT COUNT(ed.id) FROM eventdata ed JOIN eventperioddata epd ON epd.id = ed.periodId WHERE ed.type = 0 AND epd.id = ? AND ed.startDate = UTC_DATE()", periodId)
			result.Scan(&count)

			if count < 2 {
				add2kkiEventLocations(0, 2)
			}

			weekday := time.Now().UTC().Weekday()

			result = db.QueryRow("SELECT COUNT(ed.id) FROM eventdata ed JOIN eventperioddata epd ON epd.id = ed.periodId WHERE ed.type = 2 AND epd.id = ? AND ed.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday))
			result.Scan(&count)

			if count < 1 {
				add2kkiEventLocations(1, 1)
			}

			if weekday == time.Friday || weekday == time.Saturday {
				result = db.QueryRow("SELECT COUNT(ed.id) FROM eventdata ed JOIN eventperioddata epd ON epd.id = ed.periodId WHERE ed.type = 1 AND epd.id = ? AND ed.startDate = DATE_SUB(UTC_DATE(), INTERVAL ? DAY)", periodId, int(weekday)-int(time.Friday))
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
	}
}

func add2kkiEventLocations(eventType int, count int) {
	periodId, err := readCurrentEventPeriodId()
	if err != nil {
		handleInternalEventError(eventType, err)
	}

	url := "https://2kki.app/getRandomLocations?count=" + strconv.Itoa(count) + "&ignoreSecret=1"
	if eventType == 0 {
		url += "&minDepth=3&maxDepth=9"
	} else if eventType == 1 {
		url += "&minDepth=11"
	} else {
		url += "&minDepth=9&maxDepth=14"
	}

	resp, err := http.Get(url)
	if err != nil {
		handleInternalEventError(eventType, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		handleInternalEventError(eventType, err)
		return
	}

	if strings.HasPrefix(string(body), "{\"error\"") {
		handleEventError(eventType, "Invalid event location data: "+string(body))
	}

	var eventLocations []EventLocationData
	err = json.Unmarshal(body, &eventLocations)
	if err != nil {
		handleInternalEventError(eventType, err)
	}

	for _, eventLocation := range eventLocations {
		err = writeEventLocationData(periodId, eventType, eventLocation.Title, eventLocation.TitleJP, eventLocation.Depth, eventLocation.MapIds)
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
