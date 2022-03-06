package server

import (
	"strings"
)

type HubController struct {
	hubs []*Hub
	config Config

	database *Database
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

func (h *HubController) isValidSpriteName(name string) bool {
	if name == "" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range h.config.spriteNames {
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func (h *HubController) isValidSystemName(name string) bool {
	for _, otherName := range h.config.systemNames {
		if strings.EqualFold(otherName, name) {
			return true
		}
	}
	return false
}

func (h *HubController) isValidSoundName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	for _, otherName := range h.config.soundNames {
		if strings.EqualFold(otherName, name) {
			for _, ignoredName := range h.config.ignoredSoundNames {
				if strings.EqualFold(ignoredName, name) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func (h *HubController) isValidPicName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	nameLower := strings.ToLower(name)
	for _, otherName := range h.config.pictureNames {
		if otherName == nameLower {
			return true
		}
	}
	for _, prefix := range h.config.picturePrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return true
		}
	}

	return false
}
