package server

type RankingCategory struct {
	CategoryId    string               `json:"categoryId"`
	Game          string               `json:"game"`
	SubCategories []RankingSubCategory `json:"subCategories"`
}

type RankingSubCategory struct {
	SubCategoryId string `json:"subCategoryId"`
	Game          string `json:"game"`
}

type RankingEntry struct {
	Position   int
	Uuid       string
	ValueInt   int
	ValueFloat float32
}

type Ranking struct {
	Position   int     `json:"position"`
	Name       string  `json:"name"`
	Rank       int     `json:"rank"`
	Badge      string  `json:"badge"`
	ValueInt   int     `json:"valueInt"`
	ValueFloat float32 `json:"valueFloat"`
}

func StartRankings() {
	// Will be used to initialize and update rankings
	/*s := gocron.NewScheduler(time.UTC)

	var rankingCategories []*RankingCategory

	if len(badges) > 0 {
		badgeCountCategory := &RankingCategory{CategoryId: "badgeCount"}
		rankingCategories = append(rankingCategories, badgeCountCategory)

		badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: "all"})
		if _, ok := badges[config.gameName]; ok {
			badgeCountCategory.SubCategories = append(badgeCountCategory.SubCategories, RankingSubCategory{SubCategoryId: config.gameName, Game: config.gameName})
		}
	}

	s.StartAsync()*/
}
