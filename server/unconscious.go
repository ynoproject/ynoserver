package server

import (
	"math"
	"math/rand/v2"
	"time"
)

var startTime = time.Now()
var randint, temperature, precipitation int

func initUnconscious() {
	scheduler.Every(1).Minute().Do(func() {
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
	scheduler.Every(2).Minutes().Do(func() {
		temperature = max(-100, min(100, temperature+weatherDelta(temperature)))
		precipitation = max(0, min(100, precipitation+weatherDelta(precipitation)))
		for _, client := range clients.Get() {
			if client.roomC == nil {
				continue
			}

			select {
			case client.roomC.outbox <- buildMsg("cuw", temperature, precipitation):
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
	return int(time.Since(startTime).Minutes()) % 240
}

func didJoinRoomUnconscious(c *RoomClient) {
	if c == nil {
		return
	}

	c.outbox <- buildMsg("cut", getUnconsciousTime(), randint)
	c.outbox <- buildMsg("cuw", temperature, precipitation)
}
