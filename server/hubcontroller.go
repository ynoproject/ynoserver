package server

type HubController struct {
	hubs []*Hub
}

func (h *HubController) addHub(roomName string) {
	hub := NewHub(roomName, h)
	h.hubs = append(h.hubs, hub)
	go hub.Run()
}

func (h *HubController) globalBroadcast(inpData []byte) {
	for _, hub := range h.hubs {
		hub.broadcast(inpData)
	}
}
