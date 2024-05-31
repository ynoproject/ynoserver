package server

import "time"

var startTime = time.Now()

func initUnconscious() {
	scheduler.Every(1).Minute().Do(func() {
		for _, client := range clients.Get() {
			if client.roomC != nil {
				client.roomC.handleCut()
			}
		}
	})
}

func getUnconsciousTime() int {
	return int(time.Since(startTime).Minutes()) % 240
}
