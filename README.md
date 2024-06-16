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

## Deployment

Prepare the following 2 environment variables:

- `USER_TOKEN`: GitHub Personal Access Token (PAT) with the `repo` scope.
- `WEBHOOK_SECRET`: GitHub webhook secret.

Then, deploy the service to the server with the following command:

```sh
$ docker compose up -d
```

The service will be available at `http://<host>:8080`, and the webhook endpoint will be `http://<host>:8080/event`.

## License

MIT
