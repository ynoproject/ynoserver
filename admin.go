package main

func tokenIsRank(token string, rank int) bool {
	_, _, playerRank, _, _, _ := readPlayerDataFromToken(token)

	return playerRank >= rank
}
