package rssmonitor

import (
	"testing"
	"time"

	"enc/utils/tele"

	"github.com/celestix/gotgproto/types"
	"github.com/gotd/td/tg"
)

func Test_fakeEventHandler(t *testing.T) {
	// 1. Setup a channel to receive results (including args and error status)
	type handlerResult struct {
		name      string
		argsFound string
		err       error
	}
	resultsChan := make(chan handlerResult, 2)

	// 2. Mock Handlers that send precise results back to the main test thread
	HandlerMap["cmd1"] = func(event *tele.Message, args string) error {
		resultsChan <- handlerResult{name: "cmd1", argsFound: args, err: nil}
		return nil
	}

	HandlerMap["cmd2"] = func(event *tele.Message, args string) error {
		resultsChan <- handlerResult{name: "cmd2", argsFound: args, err: nil}
		return nil
	}
	// Note: Add t.Cleanup logic here as shown in the previous answer to reset HandlerMap

	tests := []*tele.Message{
		{
			Raw: &types.Message{ // Assuming types/tg imports from your setup
				Message: &tg.Message{
					Message: "/cmd1 data",
				},
			},
		},
		{
			Raw: &types.Message{
				Message: &tg.Message{
					Message: "/cmd2 data",
				},
			},
		},
	}

	// Total expected calls
	expectedCalls := len(tests)

	// 3. Loop and trigger the events (these calls return immediately)
	for _, event := range tests {
		fakeEventHandler(event)
	}

	// 4. Verification Stage: Wait for results using a timeout
	timeout := time.After(4 * time.Second)     // Give it slightly more than the 3s sleep in production code
	receivedResults := make(map[string]string) // To keep track of which cmds finished and their args

	for range expectedCalls {
		select {
		case result := <-resultsChan:
			if result.err != nil {
				t.Errorf("Handler %s reported an error: %v", result.name, result.err)
			}
			receivedResults[result.name] = result.argsFound

		case <-timeout:
			t.Fatalf("Test timed out waiting for handlers to execute. Expected %d calls, received %d.",
				expectedCalls, len(receivedResults))
		}
	}

	// 5. Final Assertions on the collected data (fails if args are wrong)

	// Check cmd1 arguments
	if args, found := receivedResults["cmd1"]; !found {
		t.Error("Handler for cmd1 was never called.")
	} else if args != "data" {
		t.Errorf("cmd1 handler received wrong arguments. Got: %q, Expected: %q", args, "data")
	}

	// Check cmd2 arguments
	if args, found := receivedResults["cmd2"]; !found {
		t.Error("Handler for cmd2 was never called.")
	} else if args != "data" {
		t.Errorf("cmd2 handler received wrong arguments. Got: %q, Expected: %q", args, "data")
	}

	// If you reach here, all handlers were called with the correct arguments.
	t.Log("All handlers executed successfully with correct arguments.")
}
