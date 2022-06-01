package main

type MinigameConfig struct {
	MinigameId     string `json:"minigameId"`
	VarId          int    `json:"varId"`
	InitialVarSync bool   `json:"initialVarSync"`
	SwitchId       int    `json:"switchId"`
	SwitchValue    bool   `json:"switchValue"`
}

func getHubMinigameConfigs(roomName int) (minigameConfigs []*MinigameConfig) {
	switch config.gameName {
	case "yume":
		if roomName == 155 {
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "nasu", VarId: 88, SwitchId: 215})
		}
	case "2kki":
		switch roomName {
		case 102:
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "rby", VarId: 1010, InitialVarSync: true})
		}
	}
	return minigameConfigs
}
