package server

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

func setWordFilter() error {
	data, err := os.Open("filterwords.txt")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(data)

	scanner.Split(bufio.ScanLines)

	var regexStr strings.Builder
	regexStr.WriteString("(?i)(")

	var wordAdded bool
	for scanner.Scan() {
		if wordAdded {
			regexStr.WriteString("|")
		}

		regexStr.WriteString(scanner.Text())

		wordAdded = true
	}

	regex, err := regexp.Compile(regexStr.String() + ")")
	if err != nil {
		return err
	}

	wordFilter = regex

	return nil
}
