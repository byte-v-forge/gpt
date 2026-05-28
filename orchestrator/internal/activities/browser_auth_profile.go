package activities

import (
	"strconv"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v6"
)

func browserAuthRandomDisplayName() string {
	return gofakeit.Name()
}

func browserAuthRandomAge() string {
	return strconv.Itoa(gofakeit.Number(18, 22))
}

func browserAuthRandomBirthdateForAge(age int) string {
	if age < 18 {
		age = 18
	}
	if age > 22 {
		age = 22
	}
	now := time.Now().UTC()
	year := now.Year() - age
	maxDay := now.YearDay()
	if maxDay < 1 {
		maxDay = 1
	}
	birthday := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, gofakeit.Number(0, maxDay-1))
	return birthday.Format("2006-01-02")
}

func compactStringValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
