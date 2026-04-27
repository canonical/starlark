# Security Policy

## Risks

- The user's script uses a large amount of system memory when executed (caused by improperly-bounded builtins)
- The user's script takes a very long time to run, starving other components of execution time (caused by improperly-bounded builtins)
- The user's script accesses data outside of the sandbox (caused by improperly-bounded builtins)
- Sensitive data is exposed

## Good practices

- Use the `startest` framework to test safety properties of all exposed builtins
- Bound the size of a user's script sets
- If sensitive values must be exposed:
  - Limit users who can access it by adding permission checks in related builtins
  - Use custom types (which implement `starlark.Value`) to avoid sensitive data accidentally ending up in logs
  - Restrict access to logs

## Deployment checklist

- [ ] All exposed builtins abide by all safety properties defined in Starlark (memory usage is accounted, execution time is bounded etc.)
- [ ] Script set size is externally bounded
- [ ] Secrets or properties of secrets are not be visible in logs (e.g. via `print(secret)`)

## Reporting a Vulnerability

To report a security issue, please follow the steps below:

Using GitHub, file a [Private Security Report](https://github.com/canonical/starlark/security/advisories/new) with:

- A description of the issue
- Steps to reproduce the issue
- Affected versions of the `starlark` package
- Any known mitigations for the issue

The [Ubuntu Security disclosure and embargo policy](https://ubuntu.com/security/disclosure-policy) contains more information about what to expect during this process and our requirements for responsible disclosure.

Thank you for contributing to the security and integrity of the `starlark`!
