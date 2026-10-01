# Security Policy

## Reporting a vulnerability

Please report security issues privately using GitHub's [Security Advisory](https://github.com/caseyrobb/kubescrub/security/advisories/new) feature. **Do not open a public issue** for security concerns.

We aim to acknowledge reports within 48 hours and release a fix as soon as possible.

## Operational security

- **Apply must only be pointed at clusters you own.** The `--apply --yes` flag performs real deletions. Misconfiguration can cause unintended resource removal.
- **Never reuse the scan and apply ServiceAccounts.** The scan identity (`kubescrub-scan`) provides read-only access. The apply identity (`kubescrub-apply`) grants delete permissions on jobs, replicasets, and PVCs. They are separate ClusterRoles and ServiceAccounts and must not share the same binding.
- **Scan with the scan ServiceAccount for least privilege.** When using the scan command with a user kubeconfig, you have full read access to the context. Using `--as=system:serviceaccount:kubescrub-system:kubescrub-scan` restricts the identity to the read-only ClusterRole.
- **The apply annotation is mandatory.** KubeScrub will not delete any resource that lacks the `kubescrub.io/allow-delete=true` annotation, even when `--apply --yes` is used.
