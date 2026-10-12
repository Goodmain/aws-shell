# Smoke test setup

Use a non-production AWS account profile with read-only ECS permissions.

1. Confirm an authentication provider is installed and the profile works:

   ```bash
   aws-vault exec <profile> -- aws sts get-caller-identity
   # or
   aws sts get-caller-identity --profile <profile>
   ```

2. Export the profile for the integration smoke test. For AWS SSO, also set `AWS_PROFILE` so credentialed mode inherits it:

   ```bash
   export AWS_SHELL_SMOKE_PROFILE=<profile>
   export AWS_PROFILE=<profile> # AWS SSO only
   ```

3. Run the smoke test manually:

   ```bash
   go test ./test/smoke -tags=integration -v
   ```
