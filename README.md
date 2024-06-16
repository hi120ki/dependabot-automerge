# dependabot-automerge

A service that automatically approves and merges dependabot PRs.

## Usage

1. Create a new Classic PAT (Personal Access Token) with the `repo` scope.
2. Add the PAT to the environment variable `USER_TOKEN`.
3. Prepare the webhook secret by GitHub App or Organization webhook and enable the webhook with the `Pull request` event.
4. Add the webhook secret to the environment variable `WEBHOOK_SECRET`.
5. Make `config.yml` file and list the repositories you want to enable the autoapprove and automerge feature.
6. Configure the webhook URL (`/event`) to GitHub.
