package utils

import (
	"slices"
	"strings"

	"enc/config"
	"enc/internal/bot"
)

func UserIsAllowed(userID int64) bool {
	return (UserIsOwner(userID) || UserIsTemp(userID))
}

func UserIsOwner(userID int64) bool {
	return slices.Contains(config.Rconf.Owners, userID)
}

func UserIsTemp(userID int64) bool {
	return slices.Contains(bot.TempUsers, userID)
}

func UserIsDev(userID int64) bool {
	return userID == config.Conf.Dev
}

func AllowPM() bool {
	return !config.Conf.NoPM
}

func AddTempUser(userID int64) {
	bot.TempUsers = append(bot.TempUsers, userID)
}

func RmTempUser(userID int64) {
	bot.TempUsers = remove(bot.TempUsers, userID)
}

func SendHelp(args string) bool {
	return strings.EqualFold(args, "-h") || strings.EqualFold(args, "--help")
}

func remove(s []int64, r int64) []int64 {
	out := s[:0]
	for _, v := range s {
		if v != r {
			out = append(out, v)
		}
	}
	return out
}

func CodeBlock(s string) string {
	return "```" + s + "```"
}

func StringFormat(s ...string) string {
	var sb strings.Builder
	for _, x := range s {
		sb.WriteString(x)
		if !strings.HasSuffix(x, " ") {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func YesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
