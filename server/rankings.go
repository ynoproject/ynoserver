package server

import (
	"time"

	"github.com/go-co-op/gocron"
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

type RankingEntryBase struct {
	Position   int     `json:"position"`
	ValueInt   int     `json:"valueInt"`
	ValueFloat float32 `json:"valueFloat"`
}

type RankingEntry struct {
	RankingEntryBase
	Uuid string `json:"uuid"`
}

type Ranking struct {
	RankingEntryBase
	Name       string `json:"name"`
	Rank       int    `json:"rank"`
	Badge      string `json:"badge"`
	SystemName string `json:"systemName"`
}

func StartRankings() {
	s := gocron.NewScheduler(time.UTC)

	var rankingCategories []*RankingCategory

	if len(badges) > 0 {
		badgeCountCategory := &RankingCategory{CategoryId: "badgeCount"}
		rankingCategories = append(rankingCategories, badgeCountCategory)

		badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: "all"})
		if _, ok := badges[config.gameName]; ok {
			// Badge records needed for determining badge game
			writeGameBadges()
			badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: config.gameName, Game: config.gameName})
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

	s.Every(1).Hour().Do(func() {
		for _, category := range rankingCategories {
			for _, subCategory := range category.SubCategories {
				// Use 2kki server to update 'all' rankings
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
