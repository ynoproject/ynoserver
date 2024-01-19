package server

import (
	"bufio"
	"os"
	"strings"
)

func setWordFilter() error {
	data, err := os.Open("filterwords.txt")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(data)

	scanner.Split(bufio.ScanLines)

	var wordFilterArgs []string
	for scanner.Scan() {
		wordFilterArgs = append(wordFilterArgs, scanner.Text())
		wordFilterArgs = append(wordFilterArgs, strings.Repeat("?", len(scanner.Text())))
	}

	wordFilter = strings.NewReplacer(wordFilterArgs...)

	return nil
}
