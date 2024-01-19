package server

import (
	"bufio"
	"os"
	"regexp"
)

func setWordFilter() error {
	data, err := os.Open("filterwords.txt")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(data)

	scanner.Split(bufio.ScanLines)

	regexStr := "(?i)("

	var wordAdded bool
	for scanner.Scan() {
		if wordAdded {
			regexStr += "|"
		}

		regexStr += scanner.Text()

		wordAdded = true
	}

	regex, err := regexp.Compile(regexStr + ")")
	if err != nil {
		return err
	}

	wordFilter = regex

	return nil
}
