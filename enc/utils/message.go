package utils

import (
	"slices"
	"strconv"
	"strings"

	"enc/config"
	"enc/internal/bot"
)

func UserIsAllowed(userID int64) bool {
	return (UserIsOwner(userID) || UserIsTemp(userID))
}

func UserIsOwner(userID int64) bool {
	str := strconv.FormatInt(userID, 10)
	return strings.Contains(config.Conf.Owner, str)
}

func UserIsTemp(userID int64) bool {
	str := strconv.FormatInt(userID, 10)
	return slices.Contains(bot.TempUsers, str)
}

func UserIsDev(userID int64) bool {
	return userID == config.Conf.Dev
}

func AllowPM() bool {
	return !config.Conf.NoPM
}

func AddTempUser(userID int64) {
	str := strconv.FormatInt(userID, 10)
	bot.TempUsers = append(bot.TempUsers, str)
}

func RmTempUser(userID int64) {
	str := strconv.FormatInt(userID, 10)
	bot.TempUsers = remove(bot.TempUsers, str)
}

func SendHelp(args string) bool {
	return strings.EqualFold(args, "-h") || strings.EqualFold(args, "--help")
}

func remove(s []string, r string) []string {
	out := s[:0]
	for _, v := range s {
		if v != r {
			out = append(out, v)
		}
	}
	return out
}

func CodeBlock(s string) string {
	return "```\n" + s + "\n```"
}