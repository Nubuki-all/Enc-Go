// Package entimer houses the bot's custom timer
package entimer

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"enc/config"
	"enc/internal/bot"
	"enc/internal/bot/state"
	"enc/internal/logger"
	"enc/utils/tele"
)

var Timer = NewEnTimer()

// EnTimer provides pause and countdown functionality
type EnTimer struct {
	mu       sync.Mutex
	indPause bool
	timeLeft int
	msgs     []*tele.Message
	once     sync.Once
}

// NewEnTimer creates an EnTimer. It uses bot.LockOnStartup initial state.
func NewEnTimer() *EnTimer {
	return &EnTimer{
		indPause: config.Conf.LockOnStartup,
		timeLeft: 0,
		msgs:     nil,
	}
}

// Start launches the timer loop once
func (et *EnTimer) Start() {
	et.once.Do(func() {
		go et.loop()
	})
}

// loop runs continuously to manage pause/countdown
func (et *EnTimer) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		et.mu.Lock()
		paused := et.indPause || et.timeLeft > 0

		if paused {
			status := "Paused "
			if et.indPause {
				status += "Indefinitely!"
			} else {
				status += "for " + fmt.Sprint(et.timeLeft) + "s"
			}
			fmt.Println(status)
			// zap.L().Info(status)

			if et.timeLeft > 0 {
				et.timeLeft--
				if et.timeLeft == 0 {
					et.indPause = false
				}
			}
			if et.timeLeft == 0 && !et.indPause {
				bot.Paused.Set(state.DefaultPause, false)
			}
			et.mu.Unlock()
			continue
		}
		et.mu.Unlock()

		// If msgs stored and no longer paused/counting
		et.mu.Lock()
		if len(et.msgs) > 0 {
			for _, m := range et.msgs {
				if _, err := m.EditText("**Timer ended or cancelled and bot has been unpaused.**"); err != nil {
					logger.WarnW("EnTimer: message edit failed", zap.Error(err))
				}
			}
			et.msgs = nil
		}
		et.mu.Unlock()
	}
}

// NewTimer starts a new countdown for duration seconds, storing optional msgs to edit upon end
func (et *EnTimer) NewTimer(duration int, msgs []*tele.Message) {
	et.mu.Lock()
	defer et.mu.Unlock()
	if !et.indPause && et.timeLeft == 0 {
		bot.Paused.Set(state.DefaultPause, true)
	}
	et.timeLeft = duration
	et.msgs = msgs
}

// PauseIndefinitely sets indefinite pause, with optional msgs
func (et *EnTimer) PauseIndefinitely(msgs []*tele.Message) {
	et.mu.Lock()
	defer et.mu.Unlock()
	if !et.indPause && et.timeLeft == 0 {
		bot.Paused.Set(state.DefaultPause, true)
	}
	et.indPause = true
	et.timeLeft = 0
	et.msgs = msgs
}

// StopTimer stops any countdown and unpauses immediately
func (et *EnTimer) StopTimer() {
	et.mu.Lock()
	defer et.mu.Unlock()
	if !et.indPause && et.timeLeft == 0 {
		return
	}
	et.timeLeft = 0
	et.indPause = false
	// et.msgs = nil
	bot.Paused.Set(state.DefaultPause, false)
}
