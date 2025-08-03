package random

import (
	"math/rand"
)

// Enmoji returns a random “online” emoji.
func Enmoji() string {
	emojis := []string{
		"🤓", "😎", "🤠", "🌚", "🛰️",
		"❤️", "📡", "🥺", "🌝", "☺️",
		"😊", "😑", "📌", "🥸", "😵‍💫",
		"🪩", "✅",
	}
	return emojis[rand.Intn(len(emojis))]
}

// Enmoji2 returns a random “offline/dead” emoji.
func Enmoji2() string {
	emojis := []string{
		"❌", "❎", "✖️", "☠️", "💀",
		"🧟", "💔", "⚰️", "⛔", "🫠",
	}
	return emojis[rand.Intn(len(emojis))]
}

// EnHearts returns a random heart emoji.
func EnHearts() string {
	hearts := []string{
		"♥️", "❤️", "💛", "💚", "🩷",
		"💜", "🩵", "💙", "🤎", "🧡",
		"🩶", "🖤",
	}
	return hearts[rand.Intn(len(hearts))]
}
