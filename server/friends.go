package server

import "errors"

func addPlayerFriend(uuid string, targetUuid string) error {
	if uuid == targetUuid {
		return errors.New("attempted adding self as friend")
	}

	var accepted bool

	results, err := db.Exec("UPDATE playerFriends SET accepted = 1 WHERE uuid = ? AND targetUuid = ?", targetUuid, uuid)
	if err == nil {
		updatedRows, err := results.RowsAffected()
		if err != nil {
			return err
		}
		accepted = updatedRows > 1
	}

	_, err = db.Exec("INSERT IGNORE INTO playerFriends (uuid, targetUuid, accepted) VALUES (?, ?, ?)", uuid, targetUuid, accepted)
	if err != nil {
		return err
	}

	return nil
}

func removePlayerFriend(uuid string, targetUuid string) error {
	_, err := db.Exec("DELETE FROM playerFriends WHERE (uuid = ? AND targetUuid = ?) OR (uuid = ? AND targetUuid = ?)", uuid, targetUuid, targetUuid, uuid)
	if err != nil {
		return err
	}

	return nil
}

func getPlayerFriendData(uuid string) (playerFriends []*PlayerListFullData, err error) {
	results, err := db.Query("SELECT pf.uuid, COALESCE(a.user, pgd.name), pd.rank, COALESCE(a.badge, ''), pgd.systemName, pgd.spriteName, pgd.spriteIndex, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM playerFriends pf JOIN playerGameData pgd ON pgd.uuid = pf.uuid JOIN players pd ON pd.uuid = pgd.uuid JOIN accounts a ON a.uuid = pd.uuid WHERE pf.uuid = ? AND pgd.game = ? ORDER BY CASE WHEN p.owner = pf.uuid THEN 0 ELSE 1 END, pd.rank DESC, pf.id", uuid, config.gameName)
	if err != nil {
		return playerFriends, err
	}

	defer results.Close()

	for results.Next() {
		playerFriend := &PlayerListFullData{
			Account:   true,
			MapId:     "0000",
			PrevMapId: "0000",
		}

		err := results.Scan(&playerFriend.Uuid, &playerFriend.Name, &playerFriend.Rank, &playerFriend.Badge, &playerFriend.SystemName, &playerFriend.SpriteName, &playerFriend.SpriteIndex, &playerFriend.Medals[0], &playerFriend.Medals[1], &playerFriend.Medals[2], &playerFriend.Medals[3], &playerFriend.Medals[4])
		if err != nil {
			return playerFriends, err
		}

		playerFriend.Online = clients.Exists(playerFriend.Uuid)

		playerFriends = append(playerFriends, playerFriend)
	}

	return playerFriends, nil
}
