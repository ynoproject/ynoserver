package server

import "time"

var startTime = time.Now()

func initUnconscious() {
	scheduler.Every(1).Minute().Do(func() {
		for _, client := range clients.Get() {
			if client.roomC == nil {
				continue
			}

			select {
			case client.roomC.outbox <- buildMsg("cut", getUnconsciousTime()):
			default:
				writeErrLog(client.uuid, client.roomC.mapId, "send channel is full")
			}
		}
	})
}

func getUnconsciousTime() int {
	return int(time.Since(startTime).Minutes()) % 240
}
