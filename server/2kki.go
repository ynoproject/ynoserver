package server

import "strings"

func isValid2kkiSprite(name string, room int) bool {
	if serverConfig.GameName == "2kki" &&
		!(strings.Contains(name, "syujinkou") ||
			strings.Contains(name, "effect") ||
			strings.Contains(name, "yukihitsuji_game") ||
			strings.Contains(name, "zenmaigaharaten_kisekae") ||
			strings.Contains(name, "主人公") ||
			name == "kodomo_04-1" ||
			name == "RioCharset16" ||
			name == "#null") ||
		strings.Contains(name, "zenmaigaharaten_kisekae") && room != 176 {
		return false
	}

	return true
}
