/*
	Copyright (C) 2021-2022  The YNOproject Developers

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"strconv"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
)

var (
	rankingsMtx sync.RWMutex
)

type RankingCategory struct {
	CategoryId    string               `json:"categoryId"`
	Game          string               `json:"game"`
	SubCategories []RankingSubCategory `json:"subCategories"`
}

type RankingSubCategory struct {
	SubCategoryId string `json:"subCategoryId"`
	Game          string `json:"game"`
	PageCount     int    `json:"pageCount"`
}

type Ranking struct {
	Position   int     `json:"position"`
	Name       string  `json:"name"`
	Rank       int     `json:"rank"`
	Badge      string  `json:"badge"`
	SystemName string  `json:"systemName"`
	ValueInt   int     `json:"valueInt"`
	ValueFloat float32 `json:"valueFloat"`
}

func initRankings() {
	s := gocron.NewScheduler(time.UTC)

	var rankingCategories []*RankingCategory

	if len(badges) > 0 {
		bpCategory := &RankingCategory{CategoryId: "bp"}
		rankingCategories = append(rankingCategories, bpCategory)

		badgeCountCategory := &RankingCategory{CategoryId: "badgeCount"}
		rankingCategories = append(rankingCategories, badgeCountCategory)

		bpCategory.SubCategories = append(bpCategory.SubCategories, RankingSubCategory{SubCategoryId: "all"})
		badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: "all"})

		if _, ok := badges[config.gameName]; ok {
			// Use Yume 2kki server to update badge data
			if config.gameName == "2kki" {
				// Badge records needed for determining badge game
				writeGameBadges()
				updatePlayerBadgeSlotCounts("")
			}
			bpCategory.SubCategories = append(bpCategory.SubCategories, RankingSubCategory{SubCategoryId: config.gameName, Game: config.gameName})
			badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: config.gameName, Game: config.gameName})
		}
	}

	eventPeriods, err := getEventPeriodData()
	if err != nil {
		writeErrLog("SERVER", "exp", err.Error())
	} else if len(eventPeriods) > 0 {
		expCategory := &RankingCategory{CategoryId: "exp", Game: config.gameName}
		rankingCategories = append(rankingCategories, expCategory)

		if len(eventPeriods) > 1 {
			expCategory.SubCategories = append(expCategory.SubCategories, RankingSubCategory{SubCategoryId: "all", Game: config.gameName})
		}
		for _, eventPeriod := range eventPeriods {
			expCategory.SubCategories = append(expCategory.SubCategories, RankingSubCategory{SubCategoryId: strconv.Itoa(eventPeriod.PeriodOrdinal), Game: config.gameName})
		}

		eventLocationCountCategory := &RankingCategory{CategoryId: "eventLocationCount", Game: config.gameName}
		rankingCategories = append(rankingCategories, eventLocationCountCategory)

		if len(eventPeriods) > 1 {
			eventLocationCountCategory.SubCategories = append(eventLocationCountCategory.SubCategories, RankingSubCategory{SubCategoryId: "all", Game: config.gameName})
		}
		for _, eventPeriod := range eventPeriods {
			eventLocationCountCategory.SubCategories = append(eventLocationCountCategory.SubCategories, RankingSubCategory{SubCategoryId: strconv.Itoa(eventPeriod.PeriodOrdinal), Game: config.gameName})
		}

		freeEventLocationCountCategory := &RankingCategory{CategoryId: "freeEventLocationCount", Game: config.gameName}
		rankingCategories = append(rankingCategories, freeEventLocationCountCategory)

		if len(eventPeriods) > 1 {
			freeEventLocationCountCategory.SubCategories = append(freeEventLocationCountCategory.SubCategories, RankingSubCategory{SubCategoryId: "all", Game: config.gameName})
		}
		for _, eventPeriod := range eventPeriods {
			freeEventLocationCountCategory.SubCategories = append(freeEventLocationCountCategory.SubCategories, RankingSubCategory{SubCategoryId: strconv.Itoa(eventPeriod.PeriodOrdinal), Game: config.gameName})
		}

		eventLocationCompletionCategory := &RankingCategory{CategoryId: "eventLocationCompletion", Game: config.gameName}
		rankingCategories = append(rankingCategories, eventLocationCompletionCategory)

		if len(eventPeriods) > 1 {
			eventLocationCompletionCategory.SubCategories = append(eventLocationCompletionCategory.SubCategories, RankingSubCategory{SubCategoryId: "all", Game: config.gameName})
		}
		for _, eventPeriod := range eventPeriods {
			eventLocationCompletionCategory.SubCategories = append(eventLocationCompletionCategory.SubCategories, RankingSubCategory{SubCategoryId: strconv.Itoa(eventPeriod.PeriodOrdinal), Game: config.gameName})
		}

		eventVmCountCategory := &RankingCategory{CategoryId: "eventVmCount", Game: config.gameName}
		rankingCategories = append(rankingCategories, eventVmCountCategory)

		for _, eventPeriod := range eventPeriods {
			if eventPeriod.EnableVms {
				eventVmCountCategory.SubCategories = append(eventVmCountCategory.SubCategories, RankingSubCategory{SubCategoryId: strconv.Itoa(eventPeriod.PeriodOrdinal), Game: config.gameName})
			}
		}

		if len(eventVmCountCategory.SubCategories) > 1 {
			eventVmCountCategory.SubCategories = append([]RankingSubCategory{{SubCategoryId: "all", Game: config.gameName}}, eventVmCountCategory.SubCategories...)
		}
	}

	if config.gameName == "2kki" {
		timeTrialMapIds, err := getTimeTrialMapIds()
		if err != nil {
			writeErrLog("SERVER", "timeTrial", err.Error())
		} else if len(timeTrialMapIds) > 0 {
			timeTrialCategory := &RankingCategory{CategoryId: "timeTrial", Game: config.gameName}
			rankingCategories = append(rankingCategories, timeTrialCategory)

			for _, mapId := range timeTrialMapIds {
				timeTrialCategory.SubCategories = append(timeTrialCategory.SubCategories, RankingSubCategory{SubCategoryId: strconv.Itoa(mapId), Game: config.gameName})
			}
		}
	}

	gameMinigameIds, err := getGameMinigameIds()
	if err != nil {
		writeErrLog("SERVER", "minigame", err.Error())
	} else {
		minigameCategory := &RankingCategory{CategoryId: "minigame", Game: config.gameName}
		rankingCategories = append(rankingCategories, minigameCategory)

		for _, minigameId := range gameMinigameIds {
			minigameCategory.SubCategories = append(minigameCategory.SubCategories, RankingSubCategory{SubCategoryId: minigameId, Game: config.gameName})
		}
	}

	for c, category := range rankingCategories {
		err := writeRankingCategory(category.CategoryId, category.Game, c)
		if err != nil {
			writeErrLog("SERVER", category.CategoryId, err.Error())
			continue
		}
		for sc, subCategory := range category.SubCategories {
			err = writeRankingSubCategory(category.CategoryId, subCategory.SubCategoryId, subCategory.Game, sc)
			if err != nil {
				writeErrLog("SERVER", category.CategoryId+"/"+subCategory.SubCategoryId, err.Error())
			}
		}
	}

	s.Every(15).Minute().Do(func() {
		defer rankingsMtx.Unlock()

		rankingsMtx.Lock()

		for _, category := range rankingCategories {
			for _, subCategory := range category.SubCategories {
				// Use Yume 2kki server to update 'all' rankings
				if subCategory.SubCategoryId == "all" && config.gameName != "2kki" {
					continue
				}
				err := updateRankingEntries(category.CategoryId, subCategory.SubCategoryId)
				if err != nil {
					writeErrLog("SERVER", category.CategoryId+"/"+subCategory.SubCategoryId, err.Error())
				}
			}
		}
	})

	s.StartAsync()
}
