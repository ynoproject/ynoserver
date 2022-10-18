package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func getCharSetList() map[string]bool {
	files, err := os.ReadDir(config.gameName + "/CharSet")
	if err != nil {
		panic(err)
	}

	charSets := make(map[string]bool)
	for _, file := range files {
		charSets[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return charSets
}

func getSoundList() map[string]bool {
	files, err := os.ReadDir(config.gameName + "/Sound")
	if err != nil {
		panic(err)
	}

	sounds := make(map[string]bool)
	for _, file := range files {
		sounds[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return sounds
}

func getSystemList() map[string]bool {
	files, err := os.ReadDir(config.gameName + "/System")
	if err != nil {
		panic(err)
	}

	systems := make(map[string]bool)
	for _, file := range files {
		systems[file.Name()[:len(file.Name())-len(filepath.Ext(file.Name()))]] = true
	}

	return systems
}

func getMapList() []int {
	files, err := os.ReadDir(config.gameName + "/")
	if err != nil {
		panic(err)
	}

	var maps []int
	for _, file := range files {
		if len(file.Name()) == 11 && file.Name()[7:] == ".lmu" {
			id, err := strconv.Atoi(file.Name()[3:7])
			if err != nil {
				panic(err)
			}

			maps = append(maps, id)
		}
	}

	return maps
}

func isValidSprite(name string) bool {
	if name == "" {
		return true
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	return config.spriteNames[name]
}

func isValidSystem(name string, ignoreSingleQuotes bool) bool {
	if ignoreSingleQuotes {
		name = strings.ReplaceAll(name, "'", "")
	}

	return config.systemNames[name]
}

func isValidSound(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	return config.soundNames[name]
}

func isValidPicName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}

	if config.pictureNames[name] {
		return true
	}

	for _, prefix := range config.picturePrefixes {
		if strings.HasPrefix(strings.ToLower(name), prefix) {
			return true
		}
	}

	return false
}
