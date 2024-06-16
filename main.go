package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-github/v62/github"
	"github.com/kelseyhightower/envconfig"
	"golang.org/x/oauth2"
)

type EnvVars struct {
	WebhookSecret string `envconfig:"WEBHOOK_SECRET" required:"true"`
	UserToken     string `envconfig:"USER_TOKEN" required:"true"`
}

type Config struct {
	Repository  string `json:"repository"`
	Autoapprove bool   `json:"autoapprove"`
	Automerge   bool   `json:"automerge"`
}

type Action struct {
	client  *github.Client
	configs []*Config
	logger  *slog.Logger
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	env, err := loadEnvVars()
	if err != nil {
		logger.Error("failed to load environment variables", "error", err)
		return
	}

	client := createGitHubClient(ctx, env.UserToken)

	configs, err := loadConfigs("config.yaml")
	if err != nil {
		logger.Error("failed to load configurations", "error", err)
		return
	}

	mux := setupHTTPHandler(ctx, client, configs, env.WebhookSecret, logger)
	if err := http.ListenAndServe(":8080", mux); err != nil {
		logger.Error("failed to start server", "error", err)
	}
}

func loadEnvVars() (*EnvVars, error) {
	var env EnvVars
	err := envconfig.Process("", &env)
	return &env, err
}

func createGitHubClient(ctx context.Context, userToken string) *github.Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: userToken})
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func loadConfigs(path string) ([]*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var configs []*Config
	err = yaml.NewDecoder(file).Decode(&configs)
	return configs, err
}

func setupHTTPHandler(ctx context.Context, client *github.Client, configs []*Config, webhookSecret string, logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/event", func(w http.ResponseWriter, r *http.Request) {
		handleWebhook(ctx, client, configs, webhookSecret, logger, w, r)
	})
	return mux
}

func handleWebhook(ctx context.Context, client *github.Client, configs []*Config, webhookSecret string, logger *slog.Logger, w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(webhookSecret))
	if err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		logger.Error("failed to validate payload", "error", err)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		http.Error(w, "Failed to parse webhook", http.StatusBadRequest)
		logger.Error("failed to parse webhook", "error", err)
		return
	}

	switch event := event.(type) {
	case *github.PullRequestEvent:
		handlePullRequestEvent(ctx, client, configs, logger, w, event)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func handlePullRequestEvent(ctx context.Context, client *github.Client, configs []*Config, logger *slog.Logger, w http.ResponseWriter, event *github.PullRequestEvent) {
	logger = logger.With(
		slog.Group(
			"pull_request",
			"owner", event.GetRepo().GetOwner().GetLogin(),
			"repository", event.GetRepo().GetName(),
			"pull_request_number", event.PullRequest.GetNumber(),
			"action", event.GetAction(),
			"actor", event.GetPullRequest().GetUser().GetLogin(),
			"url", event.GetPullRequest().GetHTMLURL(),
		),
	)

	action := &Action{client: client, configs: configs, logger: logger}

	if event.GetAction() == "opened" || event.GetAction() == "synchronize" || event.GetAction() == "reopened" {
		if err := action.run(ctx, event); err != nil {
			http.Error(w, "Failed to process pull request event", http.StatusInternalServerError)
			logger.Error("failed to run action", "error", err)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (a *Action) run(ctx context.Context, event *github.PullRequestEvent) error {
	repoName := event.Repo.GetName()
	for _, config := range a.configs {
		if config.Repository == repoName {
			a.logger.Info("running action")
			time.Sleep(15 * time.Second)

			if isOpen, err := a.checkPRisOpen(ctx, event); err != nil || !isOpen {
				return err
			}

			if err := a.verifyDependabot(ctx, event); err != nil {
				return err
			}
			a.logger.Info("verified dependabot")

			if res, err := a.verifyChecks(ctx, event); err != nil || !res {
				if err := a.rebase(ctx, event); err != nil {
					return err
				}
				a.logger.Info("rebased PR")

				if res, err := a.verifyChecks(ctx, event); err != nil || !res {
					return fmt.Errorf("checks are not passing")
				}
			}
			a.logger.Info("verified checks")

			if config.Autoapprove {
				if err := a.approve(ctx, event); err != nil {
					return err
				}
				a.logger.Info("approved PR")
			}

			if config.Automerge {
				if err := a.merge(ctx, event); err != nil {
					return err
				}
				a.logger.Info("merged PR")
			}
		}
	}
	return nil
}

func (a *Action) checkPRisOpen(ctx context.Context, event *github.PullRequestEvent) (bool, error) {
	pr := event.GetPullRequest()
	if pr == nil {
		return false, fmt.Errorf("pull request event has no pull request")
	}

	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := pr.GetNumber()

	pr, _, err := a.client.PullRequests.Get(ctx, repoOwner, repoName, prNumber)
	if err != nil || pr.GetState() != "open" {
		return false, err
	}
	return true, nil
}

func (a *Action) verifyDependabot(ctx context.Context, event *github.PullRequestEvent) error {
	pr := event.GetPullRequest()
	if pr == nil || pr.GetUser().GetLogin() != "dependabot[bot]" {
		return nil
	}

	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := pr.GetNumber()

	commits, _, err := a.client.PullRequests.ListCommits(ctx, repoOwner, repoName, prNumber, nil)
	if err != nil {
		return err
	}

	for _, commit := range commits {
		if !commit.Commit.GetVerification().GetVerified() || commit.GetAuthor().GetLogin() != "dependabot[bot]" {
			return fmt.Errorf("not all commits are signed and authored by dependabot[bot]")
		}
	}
	return nil
}

func (a *Action) verifyChecks(ctx context.Context, event *github.PullRequestEvent) (bool, error) {
	pr := event.GetPullRequest()
	if pr == nil {
		return false, fmt.Errorf("pull request event has no pull request")
	}

	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()

	const interval = 10 * time.Second
	const maxAttempts = 12

	var checks []*github.CheckRun
	allChecksCompleted := true

	for attempt := 0; attempt < maxAttempts; attempt++ {
		checksList, _, err := a.client.Checks.ListCheckRunsForRef(ctx, repoOwner, repoName, pr.GetHead().GetSHA(), nil)
		if err != nil {
			return false, err
		}

		checks = checksList.CheckRuns
		allChecksCompleted = true
		for _, check := range checks {
			if check.GetStatus() != "completed" {
				allChecksCompleted = false
				break
			}
		}

		if allChecksCompleted {
			break
		}

		time.Sleep(interval)
	}
	return allChecksCompleted, nil
}

func (a *Action) rebase(ctx context.Context, event *github.PullRequestEvent) error {
	pr := event.GetPullRequest()
	if pr == nil {
		return fmt.Errorf("pull request event has no pull request")
	}

	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := pr.GetNumber()

	comment := &github.IssueComment{
		Body: github.String("@dependabot rebase"),
	}
	_, _, err := a.client.Issues.CreateComment(ctx, repoOwner, repoName, prNumber, comment)
	return err
}

func (a *Action) approve(ctx context.Context, event *github.PullRequestEvent) error {
	pr := event.GetPullRequest()
	if pr == nil {
		return fmt.Errorf("pull request event has no pull request")
	}

	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := pr.GetNumber()

	reviews, _, err := a.client.PullRequests.ListReviews(ctx, repoOwner, repoName, prNumber, nil)
	if err != nil {
		return err
	}
	for _, review := range reviews {
		if review.GetState() == "APPROVED" {
			return nil
		}
	}

	review := &github.PullRequestReviewRequest{
		Event: github.String("APPROVE"),
	}
	_, _, err = a.client.PullRequests.CreateReview(ctx, repoOwner, repoName, prNumber, review)
	return err
}

func (a *Action) merge(ctx context.Context, event *github.PullRequestEvent) error {
	pr := event.GetPullRequest()
	if pr == nil {
		return fmt.Errorf("pull request event has no pull request")
	}

	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := pr.GetNumber()

	comment := &github.IssueComment{
		Body: github.String("@dependabot merge"),
	}
	_, _, err := a.client.Issues.CreateComment(ctx, repoOwner, repoName, prNumber, comment)
	return err
}
