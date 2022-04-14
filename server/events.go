package server

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/go-co-op/gocron"
)

type EventLocation struct {
	Id        int       `json:"id"`
	PeriodId  int       `json:"periodId"`
	Type      int       `json:"type"`
	StartDate time.Time `json:"startDate"`
	EndDate   time.Time `json:"endDate"`
	Data      string    `json:"data"`
}

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
		handleEventError(eventType, err)
	}

	url := "https://2kki.app/getRandomLocations?count=" + strconv.Itoa(count) + "&ignoreSecret=1"
	if eventType == 0 {
		url += "&minDepth=3&maxDepth=9"
	} else if eventType == 1 {
		url += "&minDepth=11"
	} else {
		url += "&minDepth=9&maxDepth=14"
	}

	resp, err := http.Get("https://2kki.app/getRandomLocations?count=2&minDepth=3&maxDepth=9&ignoreSecret=1")
	if err != nil {
		handleEventError(eventType, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		handleEventError(eventType, err)
		return
	}

	err = writeEventData(periodId, eventType, string(body))
	if err != nil {
		handleEventError(eventType, err)
	}
}

func handleEventError(eventType int, err error) {
	writeErrLog("SERVER", strconv.Itoa(eventType), err.Error())
}
