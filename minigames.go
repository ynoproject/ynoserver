package main

type MinigameConfig struct {
	MinigameId string `json:"minigameId"`
	VarId      int    `json:"varId"`
}

func getHubMinigameConfigs(roomName string) (minigameConfigs []*MinigameConfig) {
	switch config.gameName {
	case "yume":
		if roomName == "155" {
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "nasu", VarId: 88})
		}
	case "2kki":
		switch roomName {
		case "102":
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "rby", VarId: 1010})
		}
	}
	return minigameConfigs
}
