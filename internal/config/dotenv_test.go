package config

import (
	"os"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	// Write a temporary .env file.
	content := "TEST_DOTENV_KEY=hello_world\nTEST_DOTENV_QUOTED=\"quoted value\"\nTEST_DOTENV_SINGLE='single quoted'\n"
	if err := os.WriteFile(".env.test", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(".env.test")

	LoadDotEnv(".env.test")

	if got := os.Getenv("TEST_DOTENV_KEY"); got != "hello_world" {
		t.Errorf("TEST_DOTENV_KEY = %q, want %q", got, "hello_world")
	}
	if got := os.Getenv("TEST_DOTENV_QUOTED"); got != "quoted value" {
		t.Errorf("TEST_DOTENV_QUOTED = %q, want %q", got, "quoted value")
	}
	if got := os.Getenv("TEST_DOTENV_SINGLE"); got != "single quoted" {
		t.Errorf("TEST_DOTENV_SINGLE = %q, want %q", got, "single quoted")
	}
}

func TestLoadDotEnvDoesNotOverrideExisting(t *testing.T) {
	content := "TEST_DOTENV_NO_OVERRIDE=from_file\n"
	if err := os.WriteFile(".env.test2", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(".env.test2")

	os.Setenv("TEST_DOTENV_NO_OVERRIDE", "from_env")
	defer os.Unsetenv("TEST_DOTENV_NO_OVERRIDE")

	LoadDotEnv(".env.test2")

	if got := os.Getenv("TEST_DOTENV_NO_OVERRIDE"); got != "from_env" {
		t.Errorf("TEST_DOTENV_NO_OVERRIDE = %q, want %q (env should win)", got, "from_env")
	}
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	// Should not panic or error.
	LoadDotEnv(".env.this_file_does_not_exist_ever")
}
