package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAWSSettingsPrecedence(t *testing.T) {
	dir := t.TempDir()
	awsDir := filepath.Join(dir, ".aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	credentialsPath := filepath.Join(awsDir, "credentials")
	configPath := filepath.Join(awsDir, "config")
	if err := os.WriteFile(credentialsPath, []byte("[default]\naws_access_key_id = file-access\naws_secret_access_key = file-secret\n[work]\naws_access_key_id = work-access\naws_secret_access_key = work-secret\naws_session_token = work-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[default]\nregion = us-west-1\n[profile work]\nregion = ap-south-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": credentialsPath,
		"AWS_CONFIG_FILE":             configPath,
		"AWS_ACCESS_KEY_ID":           "env-access",
		"AWS_SECRET_ACCESS_KEY":       "env-secret",
		"AWS_REGION":                  "eu-central-1",
	}
	getenv := func(key string) string { return env[key] }
	rt := &runtime{getenv: getenv, homeDir: func() (string, error) { return dir, nil }}

	settings, err := rt.resolveAWSSettings(awsFlags{profile: "work", region: "us-east-2"})
	if err != nil {
		t.Fatal(err)
	}
	if settings.AccessKey != "env-access" || settings.SecretKey != "env-secret" {
		t.Fatalf("expected env credentials, got %+v", settings)
	}
	if settings.Region != "us-east-2" {
		t.Fatalf("expected flag region, got %s", settings.Region)
	}
	if settings.SessionToken != "work-token" {
		t.Fatalf("expected session token from profile, got %q", settings.SessionToken)
	}
}

func TestResolveAWSSettingsFromFiles(t *testing.T) {
	dir := t.TempDir()
	awsDir := filepath.Join(dir, ".aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	credentialsPath := filepath.Join(awsDir, "credentials")
	configPath := filepath.Join(awsDir, "config")
	if err := os.WriteFile(credentialsPath, []byte("[default]\naws_access_key_id = file-access\naws_secret_access_key = file-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[default]\nregion = us-west-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	getenv := func(string) string { return "" }
	rt := &runtime{getenv: getenv, homeDir: func() (string, error) { return dir, nil }}

	settings, err := rt.resolveAWSSettings(awsFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if settings.AccessKey != "file-access" || settings.SecretKey != "file-secret" || settings.Region != "us-west-2" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}

func TestResolveAWSSettingsIgnoresBrokenFilesWhenEnvSuffices(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": dir,
		"AWS_CONFIG_FILE":             dir,
		"AWS_ACCESS_KEY_ID":           "env-access",
		"AWS_SECRET_ACCESS_KEY":       "env-secret",
		"AWS_REGION":                  "us-east-1",
	}
	getenv := func(key string) string { return env[key] }
	rt := &runtime{getenv: getenv, homeDir: func() (string, error) { return dir, nil }}

	settings, err := rt.resolveAWSSettings(awsFlags{})
	if err != nil {
		t.Fatal(err)
	}
	if settings.AccessKey != "env-access" || settings.SecretKey != "env-secret" || settings.Region != "us-east-1" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
}
