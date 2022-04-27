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

type RankingEntry struct {
	Position   int
	Uuid       string
	ValueInt   int
	ValueFloat float32
}

type Ranking struct {
	Position   int    `json:"position"`
	Name       string `json:"name"`
	Rank       int    `json:"rank"`
	Badge      string `json:"badge"`
	SystemName string `json:"systemName"`
}

type RankingInt struct {
	Ranking
	Value int `json:"value"`
}

type RankingFloat struct {
	Ranking
	Value float32 `json:"value"`
}

func StartRankings() {
	s := gocron.NewScheduler(time.UTC)

	var rankingCategories []*RankingCategory

	if len(badges) > 0 {
		badgeCountCategory := &RankingCategory{CategoryId: "badgeCount"}
		rankingCategories = append(rankingCategories, badgeCountCategory)

		badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: "all"})
		if _, ok := badges[config.gameName]; ok {
			badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: config.gameName, Game: config.gameName})
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
					writeErrLog("SERVER", category.CategoryId+"/"+subCategory.SubCategoryId, "failed to update rankings")
				}
			}
		}
	})

	s.StartAsync()
}
