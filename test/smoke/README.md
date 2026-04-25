# Smoke test setup

Use a non-production AWS account profile with read-only ECS permissions.

1. Confirm `aws-vault` is installed and profile works:

   ```bash
   aws-vault exec <profile> -- aws sts get-caller-identity
   ```

2. Export the profile for the integration smoke test:

   ```bash
   export AWS_VAULT_SMOKE_PROFILE=<profile>
   ```

3. Run the smoke test manually:

   ```bash
   go test ./test/smoke -tags=integration -v
   ```
