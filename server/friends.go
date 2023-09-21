package server

import "errors"

type PlayerFriend struct {
	PlayerListFullData
	Accepted bool `json:"accepted"`
}

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

func getPlayerFriendData(uuid string) (playerFriends []*PlayerFriend, err error) {
	results, err := db.Query("SELECT pf.targetUuid, pf.accepted, a.user, pd.rank, COALESCE(a.badge, ''), pgd.systemName, pgd.spriteName, pgd.spriteIndex, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM playerFriends pf JOIN playerGameData pgd ON pgd.uuid = pf.targetUuid JOIN players pd ON pd.uuid = pgd.uuid JOIN accounts a ON a.uuid = pd.uuid WHERE pf.uuid = ? AND pgd.game = ? ORDER BY a.user", uuid, config.gameName)
	if err != nil {
		return playerFriends, err
	}

	defer results.Close()

	for results.Next() {
		playerListData := PlayerListData{
			Account: true,
		}
		playerFriendData := PlayerListFullData{
			PlayerListData: playerListData,
			MapId:          "0000",
			PrevMapId:      "0000",
		}
		playerFriend := &PlayerFriend{
			PlayerListFullData: playerFriendData,
		}

		err := results.Scan(&playerFriend.Uuid, &playerFriend.Accepted, &playerFriend.Name, &playerFriend.Rank, &playerFriend.Badge, &playerFriend.SystemName, &playerFriend.SpriteName, &playerFriend.SpriteIndex, &playerFriend.Medals[0], &playerFriend.Medals[1], &playerFriend.Medals[2], &playerFriend.Medals[3], &playerFriend.Medals[4])
		if err != nil {
			return playerFriends, err
		}

		if playerFriend.Accepted {
			client, ok := clients.Load(playerFriend.Uuid)
			if ok {
				if client.system != "" {
					playerFriend.SystemName = client.system
				}
				if client.sprite != "" {
					playerFriend.SpriteName = client.sprite
				}
				if client.spriteIndex > -1 {
					playerFriend.SpriteIndex = client.spriteIndex
				}

				playerFriend.Badge = client.badge
				playerFriend.Medals = client.medals

				if client.roomC != nil {
					playerFriend.MapId = client.roomC.mapId
					playerFriend.PrevMapId = client.roomC.prevMapId
					playerFriend.PrevLocations = client.roomC.prevLocations
					playerFriend.X = client.roomC.x
					playerFriend.Y = client.roomC.y
				}

				playerFriend.Online = true
			}
		}

		playerFriends = append(playerFriends, playerFriend)
	}

	return playerFriends, nil
}
