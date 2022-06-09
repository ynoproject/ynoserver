package main

type MinigameConfig struct {
	MinigameId     string `json:"minigameId"`
	VarId          int    `json:"varId"`
	InitialVarSync bool   `json:"initialVarSync"`
	SwitchId       int    `json:"switchId"`
	SwitchValue    bool   `json:"switchValue"`
}

func getHubMinigameConfigs(roomId int) (minigameConfigs []*MinigameConfig) {
	switch config.gameName {
	case "yume":
		if roomId == 155 {
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "nasu", VarId: 88, SwitchId: 215})
		}
	case "2kki":
		switch roomId {
		case 102:
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "rby", VarId: 1010, InitialVarSync: true})
		case 618:
			minigameConfigs = append(minigameConfigs, &MinigameConfig{MinigameId: "rby_ex", VarId: 79, InitialVarSync: true})
		}
	}
	return minigameConfigs
}
