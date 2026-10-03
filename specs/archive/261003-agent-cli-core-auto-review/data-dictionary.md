# Data Dictionary: agent-cli-core Auto-Review Remediation
Purpose: types introduced or changed by the remediation.

## Entities
[TBD]
## Value Objects
- auth.Token: secret held behind pointer/closure type with redacting methods (FR-002).
## Interfaces
- Hinter (output): looked up via errors.As (FR-006).
- Redactor (Config.Redactor): used by httpx errors (FR-003).
## Enumerations
- Error category auth/forbidden-host (FR-001).
## API Request/Response Types
- httpx.Config.AllowedHosts []string; plain-http opt-in flag [TBD name].
- output.FromErrorWithSecrets / FailureWithSecrets signatures [TBD].
- Typed forbidden-host error [TBD name].
