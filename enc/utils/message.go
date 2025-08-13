package utils

import (
	"slices"
	"strings"
	"strconv"

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

func TempPM() bool {
    return !config.Conf.NoTempPM
}
