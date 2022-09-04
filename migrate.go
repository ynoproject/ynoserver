package main

func handleMigrations() {
	migrateGameSavesTable()
}

func migrateGameSavesTable() {
	var cnt int
	err := db.QueryRow("SELECT COUNT(*) FROM gameSaves").Scan(&cnt)
	if err != nil {
		return // table already migrated
	}

	db.Exec("RENAME TABLE gameSaves TO playerGameSaves")
}
