package activities

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type browserAuthBirthdayParts struct {
	Month       string
	MonthPadded string
	MonthName   string
	MonthShort  string
	Day         string
	DayPadded   string
	Year        string
	US          string
}

func browserAuthBirthdayPartsFrom(value string) browserAuthBirthdayParts {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r < '0' || r > '9'
	})
	month, day, year := 1, 15, 1990
	if len(parts) >= 3 {
		first, _ := strconv.Atoi(parts[0])
		second, _ := strconv.Atoi(parts[1])
		third, _ := strconv.Atoi(parts[2])
		if len(parts[0]) == 4 {
			year, month, day = first, second, third
		} else {
			month, day, year = first, second, third
		}
	}
	if month < 1 || month > 12 {
		month = 1
	}
	if day < 1 || day > 31 {
		day = 15
	}
	if year < 1900 || year > 2100 {
		year = 1990
	}
	monthNames := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	monthName := monthNames[month-1]
	return browserAuthBirthdayParts{
		Month:       strconv.Itoa(month),
		MonthPadded: fmt.Sprintf("%02d", month),
		MonthName:   monthName,
		MonthShort:  monthName[:3],
		Day:         strconv.Itoa(day),
		DayPadded:   fmt.Sprintf("%02d", day),
		Year:        strconv.Itoa(year),
		US:          fmt.Sprintf("%02d/%02d/%04d", month, day, year),
	}
}

func browserAuthAgeFromBirthday(value string) string {
	birthday := browserAuthBirthdayPartsFrom(value)
	month, _ := strconv.Atoi(birthday.Month)
	day, _ := strconv.Atoi(birthday.Day)
	year, _ := strconv.Atoi(birthday.Year)
	now := time.Now()
	age := now.Year() - year
	if int(now.Month()) < month || (int(now.Month()) == month && now.Day() < day) {
		age--
	}
	if age < 18 || age > 100 {
		return "35"
	}
	return strconv.Itoa(age)
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
