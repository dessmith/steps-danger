package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/bitrise-io/go-steputils/command/gems"
	"github.com/bitrise-io/go-steputils/command/rubycommand"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/fileutil"
	"github.com/bitrise-io/go-utils/log"
	"github.com/kballard/go-shellquote"
)

// Config ...
type Config struct {
	RepositoryURL     string `env:"repository_url,required"`
	AdditionalOptions string `env:"additional_options"`

	GithubAPIToken   stepconf.Secret `env:"github_api_token"`
	GithubHost       string          `env:"github_host"`
	GithubAPIBaseURL string          `env:"github_api_base_url"`

	GitlabAPIToken   stepconf.Secret `env:"gitlab_api_token"`
	GitlabHost       string          `env:"gitlab_host"`
	GitlabAPIBaseURL string          `env:"gitlab_api_base_url"`

	BitbucketServerAPIToken          stepconf.Secret `env:"bitbucket_server_api_token"`
	BitbucketServerHost              string          `env:"bitbucket_server_host"`
	BitbucketServerUsername          string          `env:"bitbucket_server_username"`
	BitbucketServerGitURL            string          `env:"bitbucket_server_git_url"`
	BitbucketServerCodeInsightsKey   string          `env:"bitbucket_server_code_insights_report_key"`
	BitbucketServerCodeInsightsTitle string          `env:"bitbucket_server_code_insights_report_title"`
	BitbucketServerCodeInsightsDesc  string          `env:"bitbucket_server_code_insights_report_description"`
	BitbucketServerChangeID          string          `env:"bitbucket_server_change_id"`
	// BitbucketServerDismissOutOfRange string   `env:"bitbucket_server_dismiss_out_of_range_messages"`

}

func validateInputs(cfg Config) {
	if cfg.GithubAPIToken == "" && cfg.GitlabAPIToken == "" && cfg.BitbucketServerAPIToken == "" {
		failf("None of the API tokens have been set.  If you want to use GitHub you need to set github_api_token. If you want to use GitLab you need to set gitlab_api_token. If you want to use Bitbucket Server you need to set bitbucket_server_api_token")
	}

	// GitHub enterprise
	if (cfg.GithubHost != "" || cfg.GithubAPIBaseURL != "") && (cfg.GithubHost == "" || cfg.GithubAPIBaseURL == "") {
		failf("If you want to use GitHub Enterprise you need to set both of the github_host and the github_api_base_url")
	}

	// GitLab enterprise
	if (cfg.GitlabHost != "" || cfg.GitlabAPIBaseURL != "") && (cfg.GitlabHost == "" || cfg.GitlabAPIBaseURL == "") {
		failf("If you want to use GitLab Enterprise you need to set both of the gitlab_host and the gitlab_api_base_url")
	}

}

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

func getBundlerVersion() (gems.Version, error) {
	lockFileContent, err := fileutil.ReadStringFromFile("Gemfile.lock")
	if err != nil {
		log.Warnf("Could not read from Gemfile.lock, error: %s", err)
		log.Infof("Using unspecified bundler version")
		return gems.Version{}, nil
	}

	return gems.ParseBundlerVersion(lockFileContent)
}

func main() {
	var cfg Config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}

	cfg.RepositoryURL = trimScheme(cfg.RepositoryURL)

	stepconf.Print(cfg)
	fmt.Println()

	validateInputs(cfg)

	//
	// Set local envs for the step
	for key, value := range map[string]string{
		"GIT_REPOSITORY_URL":                                      cfg.RepositoryURL,
		"DANGER_GITHUB_API_TOKEN":                                 string(cfg.GithubAPIToken),
		"DANGER_GITHUB_HOST":                                      cfg.GithubHost,
		"DANGER_GITHUB_API_BASE_URL":                              cfg.GithubAPIBaseURL,
		"DANGER_GITLAB_API_TOKEN":                                 string(cfg.GitlabAPIToken),
		"DANGER_GITLAB_HOST":                                      cfg.GitlabHost,
		"DANGER_GITLAB_API_BASE_URL":                              cfg.GitlabAPIBaseURL,
		"DANGER_BITBUCKETSERVER_HOST":                             cfg.BitbucketServerHost,
		"DANGER_BITBUCKETSERVER_USERNAME":                         cfg.BitbucketServerUsername,
		"DANGER_BITBUCKETSERVER_PASSWORD":                         string(cfg.BitbucketServerAPIToken),
		"JENKINS_URL":                                             "https://your-jenkins.com", // this can be anything
		"GIT_URL":                                                 cfg.BitbucketServerGitURL,
		"DANGER_BITBUCKETSERVER_CODE_INSIGHTS_REPORT_KEY":         cfg.BitbucketServerCodeInsightsKey,
		"DANGER_BITBUCKETSERVER_CODE_INSIGHTS_REPORT_TITLE":       cfg.BitbucketServerCodeInsightsTitle,
		"DANGER_BITBUCKETSERVER_CODE_INSIGHTS_REPORT_DESCRIPTION": cfg.BitbucketServerCodeInsightsDesc,
		"CHANGE_ID":                                               cfg.BitbucketServerChangeID,
		// "DANGER_BITBUCKETSERVER_DISMISS_OUT_OF_RANGE_MESSAGES":    cfg.BitbucketServerDismissOutOfRange,
	} {
		if value != "" {
			if err := os.Setenv(key, value); err != nil {
				failf("Failed to set env %s, error: %s", key, err)
			}
		}
	}

	// Sets DANGER_BITBUCKETSERVER_DISMISS_OUT_OF_RANGE_MESSAGES to true
	os.Setenv("DANGER_BITBUCKETSERVER_DISMISS_OUT_OF_RANGE_MESSAGES", "true")

	//
	// Check dependencies
	log.Infof("Checking dependencies")
	log.Printf("Bundler...")

	bundlerVersion, err := getBundlerVersion()
	if err != nil {
		failf("Could not determine required bundler version, error: %s", err)
	}

	if ok, err := rubycommand.IsGemInstalled("bundler", bundlerVersion.Version); err != nil {
		failf("Failed to check bundler, error: %s", err)
	} else if !ok {
		log.Warnf(`Bundler is not installed`)
		fmt.Println()
		log.Printf("Installing Bundler")

		installBundlerCommand := gems.InstallBundlerCommand(bundlerVersion)

		log.Donef("$ %s", installBundlerCommand.PrintableCommandArgs())
		fmt.Println()

		installBundlerCommand.SetStdout(os.Stdout).SetStderr(os.Stderr)

		if err := installBundlerCommand.Run(); err != nil {
			failf("command failed, error: %s", err)
		}
	}
	log.Printf("Bundler installed")

	//
	// Danger
	fmt.Println()
	log.Infof("Installing dependencies from your gem file")

	cmd := command.New("bundle", "install")
	cmd.SetStdout(os.Stdout)
	cmd.SetStderr(os.Stderr)
	log.Printf("$ %s", cmd.PrintableCommandArgs())

	if err := cmd.Run(); err != nil {
		failf("Failed to run bundle install, error: %s", err)
	}

	fmt.Println()
	log.Infof("Running danger")

	additionalOptions, err := shellquote.Split(cfg.AdditionalOptions)
	if err != nil {
		failf("Failed to shell-quote additional options (%s): %s", cfg.AdditionalOptions, err)
	}

	cmd = command.New("go", "run", "environment-variables.go")
	cmd.SetStdout(os.Stdout)
	cmd.SetStderr(os.Stderr)
	log.Printf("$ %s", cmd.PrintableCommandArgs())

	if err := cmd.Run(); err != nil {
		failf("Failed to run bundle exec danger, error: %s", err)
	}

	cmd = command.New("bundle", append([]string{"exec", "danger"}, additionalOptions...)...)
	cmd.SetStdout(os.Stdout)
	cmd.SetStderr(os.Stderr)
	log.Printf("$ %s", cmd.PrintableCommandArgs())

	if err := cmd.Run(); err != nil {
		failf("Failed to run bundle exec danger, error: %s", err)
	}

	fmt.Println()
	log.Donef("Done")
}

// trimScheme trims the URL if danger version is <8.0.5
func trimScheme(url string) string {
	cmd := command.New("danger", "--version")
	log.Printf("$ %s", cmd.PrintableCommandArgs())

	dangerVersion, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		log.Errorf("Could not determine danger vesion: %s", err)
		return url
	}

	log.Printf("Found danger version: %s", dangerVersion)

	if shouldTrimScheme(dangerVersion) {
		return strings.TrimLeft(url, "https://")
	}

	return url
}

func shouldTrimScheme(rawDangerVersion string) bool {
	dangerVersion, err := semver.NewVersion(rawDangerVersion)
	if err != nil {
		log.Errorf("Could not parse danger vesion: %s", err)
		return false
	}

	versionConstraint, err := semver.NewConstraint("<8.0.5")
	if err != nil {
		log.Errorf("Could not parse version constraint: %s", err)
		return false
	}

	return versionConstraint.Check(dangerVersion)
}
