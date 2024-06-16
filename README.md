# dependabot-automerge

A service that automatically approves and merges dependabot PRs.

## Usage

1. Create a new Classic PAT (Personal Access Token) with the `repo` scope.
2. Add the PAT to the environment variable `USER_TOKEN`.
3. Create a new secret for webhook secret.
4. Add the webhook secret to the environment variable `WEBHOOK_SECRET`.
5. Make `config.yml` file and list the repositories you want to enable the autoapprove and automerge feature.
6. Deploy the service to the server (please refer to the deployment section).
7. Create webhook by GitHub App or Organization webhook with the `Pull request` event, and the server URL (`/event`), and the webhook secret.

## Configuration

The `config.yml` can be configured as follows:

```yaml
- repository: repository-name
  autoapprove: true
  automerge: true
- repository: another-repository-name
  autoapprove: true
  automerge: true
```

## License

MIT
