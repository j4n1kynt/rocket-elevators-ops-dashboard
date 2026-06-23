package main

import (
	"os"
	"testing"
)

// TestMain clears OPENROUTER_API_KEY for the whole package test run. The agent
// tests mock the Ollama endpoint, so an ambient OPENROUTER_API_KEY in the
// developer's shell must not flip callChatLLM to the OpenRouter provider. A test
// that needs the OpenRouter path sets the var with t.Setenv, which restores it.
func TestMain(m *testing.M) {
	os.Unsetenv("OPENROUTER_API_KEY")
	os.Exit(m.Run())
}
