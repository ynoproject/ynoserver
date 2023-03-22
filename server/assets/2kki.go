package assets

import "strings"

func (a *Assets) IsValid2kkiSprite(name string, room int) bool {
	if (allowed2kkiSprites[name] ||
		(strings.Contains(name, "syujinkou") ||
			strings.Contains(name, "effect") ||
			strings.Contains(name, "yukihitsuji_game") ||
			strings.Contains(name, "zenmaigaharaten_kisekae") ||
			strings.Contains(name, "主人公"))) &&
		!(strings.Contains(name, "zenmaigaharaten_kisekae") && room != 176) {
		return true
	}

	return true
}

var allowed2kkiSprites = map[string]bool{
	"#null":                   true,
	"kodomo_04-1":             true,
	"Kong_Urotsuki_CharsetFC": true,
	"kura CharSet01":          true,
	"kuro9-8":                 true,
	"natl_char_uro":           true,
	"nuls_sujinkou":           true,
	"RioCharset16":            true,
	"urotsuki_sniper":         true,
	"urotsuki_Swimsuit01":     true,
	"urotsuki_Swimsuit02":     true,
	"urotsuki_taoru":          true,
}
