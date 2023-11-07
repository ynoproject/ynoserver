package server

import (
	"encoding/json"
	"errors"
)

type PlayerFriend struct {
	PlayerListFullData
	Game     string `json:"game"`
	Incoming bool   `json:"incoming"`
	Accepted bool   `json:"accepted"`
}

func sendFriendsUpdate() {
	for _, client := range clients.Get() {
		if !client.account {
			continue
		}

		playerFriendData, err := getPlayerFriendData(client.uuid)
		if err != nil {
			continue
		}

		// for private mode
		onlineFriends := make(map[string]bool)

		for _, friend := range playerFriendData {
			if friend.Accepted && friend.Online {
				onlineFriends[friend.Uuid] = true
			}
		}

		client.onlineFriends = onlineFriends

		playerFriendDataJson, err := json.Marshal(playerFriendData)
		if err != nil {
			continue
		}

		client.outbox <- buildMsg("pf", playerFriendDataJson)
	}
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
		accepted = updatedRows > 0
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
	results, err := db.Query("SELECT pf.targetUuid, pf.accepted, 0, a.user, pd.rank, COALESCE(a.badge, ''), pgd.game, pgd.online, pgd.timestampLastActive, pgd.systemName, pgd.spriteName, pgd.spriteIndex, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM playerFriends pf JOIN playerGameData pgd ON pgd.uuid = pf.targetUuid JOIN players pd ON pd.uuid = pgd.uuid JOIN accounts a ON a.uuid = pd.uuid WHERE pf.uuid       = ? AND pgd.game = (SELECT rpgd.game FROM playerGameData rpgd WHERE rpgd.uuid = pf.targetUuid AND rpgd.spriteName <> '' ORDER BY online DESC, timestampLastActive DESC, CASE WHEN game = ? THEN 1 ELSE 0 END DESC LIMIT 1) UNION "+
		"                       SELECT pf.uuid,       pf.accepted, 1, a.user, pd.rank, COALESCE(a.badge, ''), pgd.game, pgd.online, pgd.timestampLastActive, pgd.systemName, pgd.spriteName, pgd.spriteIndex, pgd.medalCountBronze, pgd.medalCountSilver, pgd.medalCountGold, pgd.medalCountPlatinum, pgd.medalCountDiamond FROM playerFriends pf JOIN playerGameData pgd ON pgd.uuid = pf.uuid       JOIN players pd ON pd.uuid = pgd.uuid JOIN accounts a ON a.uuid = pd.uuid WHERE pf.targetUuid = ? AND pgd.game = (SELECT rpgd.game FROM playerGameData rpgd WHERE rpgd.uuid = pf.uuid       AND rpgd.spriteName <> '' ORDER BY online DESC, timestampLastActive DESC, CASE WHEN game = ? THEN 1 ELSE 0 END DESC LIMIT 1) AND NOT EXISTS (SELECT * FROM playerFriends opf WHERE opf.uuid = pf.targetUuid AND opf.targetUuid = pf.uuid) ORDER BY user", uuid, config.gameName, uuid, config.gameName)
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

		err := results.Scan(&playerFriend.Uuid, &playerFriend.Accepted, &playerFriend.Incoming, &playerFriend.Name, &playerFriend.Rank, &playerFriend.Badge, &playerFriend.Game, &playerFriend.Online, &playerFriend.LastActive, &playerFriend.SystemName, &playerFriend.SpriteName, &playerFriend.SpriteIndex, &playerFriend.Medals[0], &playerFriend.Medals[1], &playerFriend.Medals[2], &playerFriend.Medals[3], &playerFriend.Medals[4])
		if err != nil {
			return playerFriends, err
		}

		if playerFriend.Accepted && playerFriend.Game == config.gameName {
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
