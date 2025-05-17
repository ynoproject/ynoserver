package server

import (
	"math"
	"math/rand/v2"
	"time"
)

// 1 minute = 1 ingame hour
const (
	minutesInMillis      = 60 * 1000
	gameHoursPerGameDay  = 20
	gameDaysPerGameMonth = 12
	gameMonthInMinutes   = gameHoursPerGameDay * gameDaysPerGameMonth
	gameMonthsInMillis   = gameMonthInMinutes * minutesInMillis

	unconsciousEventsVar = 1237
)

var (
	randint, temperature, precipitation int
	midnight                            time.Time
)

func initUnconscious() {
	now := time.Now().UTC()
	midnight = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// 2 seconds backwards to account for network overhead
	scheduler.CronWithSeconds("58 */1 * * * *").Do(func() {
		randint = rand.IntN(256)
		time := getUnconsciousTime()
		for _, client := range clients.Get() {
			if client.roomC == nil {
				continue
			}

			select {
			case client.roomC.outbox <- buildMsg("cut", time, randint):
			default:
				writeErrLog(client.uuid, client.roomC.mapId, "send channel is full")
			}
		}
	})
	scheduler.CronWithSeconds("58 */2 * * * *").Do(func() {
		temperature += weatherDelta(temperature)
		precipitation += weatherDelta(precipitation)

		tempValue := max(-100, min(100, temperature))
		precipValue := max(0, min(100, precipitation))
		for _, client := range clients.Get() {
			if client.roomC == nil {
				continue
			}

			select {
			case client.roomC.outbox <- buildMsg("cuw", tempValue, precipValue):
			default:
				writeErrLog(client.uuid, client.roomC.mapId, "send channel is full")
			}
		}
	})
}

func weatherDelta(n int) int {
	var sign float64 = 1
	if n < 0 {
		sign = -1
	}
	return int(rand.Int32N(21)) - 10 + int(math.Round(math.Pow(float64(n)/100, 2))*sign*-4)
}

func getUnconsciousTime() int {
	return int(time.Now().UTC().Sub(midnight).Milliseconds() / minutesInMillis % gameMonthInMinutes)
}

func getUnconsciousEvents() (eventFlags int) {
	today := time.Now().UTC()
	_, month, date := today.Date()
	if month == time.June && 4 <= date && date <= 11 {
		eventFlags = 1
	}

	return
}

func didJoinRoomWsUnconscious(c *RoomClient) {
	if c == nil {
		return
	}

	c.outbox <- buildMsg("cut", getUnconsciousTime(), randint)
	c.outbox <- buildMsg("cuw", temperature, precipitation)
}

func didJoinRoomUnconscious(c *RoomClient) {
	if c == nil {
		return
	}

	c.outbox <- buildMsg("ssv", unconsciousEventsVar, getUnconsciousEvents())
}
