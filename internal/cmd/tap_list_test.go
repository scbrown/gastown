package cmd

import (
	"strings"
	"testing"
)

func TestBuiltInTapHandlersTeachToolNameMatchers(t *testing.T) {
	handlers := builtInTapHandlers()
	if len(handlers) == 0 {
		t.Fatal("built-in handler catalogue is empty")
	}
	for _, handler := range handlers {
		if len(handler.matchers) != 1 || handler.matchers[0] != "Bash" {
			t.Errorf("%s teaches matchers %q; self-filtering Bash guards must teach exactly Bash", handler.name, handler.matchers)
		}
		for _, matcher := range handler.matchers {
			if strings.Contains(matcher, "(") {
				t.Errorf("%s teaches dead command-pattern matcher %q", handler.name, matcher)
			}
		}
	}
}
