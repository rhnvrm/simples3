package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rhnvrm/simples3"
)

type awsFlags struct {
	profile  string
	region   string
	endpoint string
	json     bool
}

type awsSettings struct {
	Profile      string
	Region       string
	AccessKey    string
	SecretKey    string
	SessionToken string
	Endpoint     string
}

type iniSections map[string]map[string]string

func addAWSFlags(fs interface {
	StringVar(*string, string, string, string)
	BoolVar(*bool, string, bool, string)
}, flags *awsFlags) {
	fs.StringVar(&flags.profile, "profile", "", "AWS profile name")
	fs.StringVar(&flags.region, "region", "", "AWS region")
	fs.StringVar(&flags.endpoint, "endpoint", "", "custom S3 endpoint URL")
	fs.BoolVar(&flags.json, "json", false, "emit JSON output")
}

func (rt *runtime) resolveAWSSettings(flags awsFlags) (awsSettings, error) {
	homeDir, err := rt.homeDir()
	if err != nil {
		return awsSettings{}, err
	}

	profile := firstNonEmpty(flags.profile, rt.getenv("AWS_PROFILE"), "default")
	credentialsPath := firstNonEmpty(rt.getenv("AWS_SHARED_CREDENTIALS_FILE"), filepath.Join(homeDir, ".aws", "credentials"))
	configPath := firstNonEmpty(rt.getenv("AWS_CONFIG_FILE"), filepath.Join(homeDir, ".aws", "config"))
	envAccessKey := firstNonEmpty(rt.getenv("AWS_ACCESS_KEY_ID"), rt.getenv("AWS_ACCESS_KEY"))
	envSecretKey := firstNonEmpty(rt.getenv("AWS_SECRET_ACCESS_KEY"), rt.getenv("AWS_SECRET_KEY"))
	envSessionToken := rt.getenv("AWS_SESSION_TOKEN")
	envRegion := firstNonEmpty(flags.region, rt.getenv("AWS_REGION"), rt.getenv("AWS_DEFAULT_REGION"))

	credentials, err := parseINIFile(credentialsPath)
	if err != nil {
		if envAccessKey == "" || envSecretKey == "" {
			return awsSettings{}, err
		}
		credentials = iniSections{}
	}
	config, err := parseINIFile(configPath)
	if err != nil {
		if envRegion == "" {
			return awsSettings{}, err
		}
		config = iniSections{}
	}

	credsSection := credentials[profile]
	configSection := config[awsConfigSection(profile)]

	settings := awsSettings{
		Profile:      profile,
		Region:       firstNonEmpty(envRegion, configValue(configSection, "region"), "us-east-1"),
		AccessKey:    firstNonEmpty(envAccessKey, configValue(credsSection, "aws_access_key_id")),
		SecretKey:    firstNonEmpty(envSecretKey, configValue(credsSection, "aws_secret_access_key")),
		SessionToken: firstNonEmpty(envSessionToken, configValue(credsSection, "aws_session_token")),
		Endpoint:     firstNonEmpty(flags.endpoint, rt.getenv("AWS_ENDPOINT_URL_S3"), rt.getenv("AWS_ENDPOINT_URL"), rt.getenv("AWS_S3_ENDPOINT")),
	}

	if settings.AccessKey == "" || settings.SecretKey == "" {
		return awsSettings{}, errors.New("missing AWS credentials: set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY or configure an AWS credentials file")
	}

	return settings, nil
}

func (settings awsSettings) newClient() *simples3.S3 {
	client := simples3.New(settings.Region, settings.AccessKey, settings.SecretKey)
	if settings.SessionToken != "" {
		client.SetToken(settings.SessionToken)
	}
	if settings.Endpoint != "" {
		client.SetEndpoint(settings.Endpoint)
	}
	return client
}

func parseINIFile(path string) (iniSections, error) {
	sections := iniSections{}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sections, nil
		}
		return nil, err
	}
	defer file.Close()

	current := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			current = strings.TrimSpace(line[1 : len(line)-1])
			if _, ok := sections[current]; !ok {
				sections[current] = map[string]string{}
			}
			continue
		}
		if current == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		sections[current][strings.TrimSpace(strings.ToLower(key))] = trimINIValue(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return sections, nil
}

func trimINIValue(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, `"`)
	return trimmed
}

func awsConfigSection(profile string) string {
	if profile == "" || profile == "default" {
		return "default"
	}
	return "profile " + profile
}

func configValue(section map[string]string, key string) string {
	if section == nil {
		return ""
	}
	return section[strings.ToLower(key)]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
