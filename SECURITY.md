# Security Policy

The PebblePost project takes the security of our application and user data seriously. Because PebblePost processes API credentials, private endpoints, and authentication tokens, every feature is designed with a local-first, zero-leak principle.

---

## Security Architecture Guarantee

1. **Local-First Processing**: All API requests, scripts, and tokens are executed locally on your machine. PebblePost does not transmit your collections, environments, or network payloads to any external cloud service or analytics platform.
2. **Secret Isolation**: PebblePost automatically categorizes environment files:
   - `*.env.json`: Public variables designed to be committed to Git.
   - `*.secret.env.json`: Secrets and private keys that are automatically placed inside `.pebble/.gitignore`.
3. **Request Script Status**: The Rust server currently preserves JavaScript request script fields but does not execute them. Do not rely on those scripts for security controls or request behavior until the Rust sandbox is available.

---

## Supported Versions

Only the latest release receives active security patches.

| Version | Supported |
| :--- | :--- |
| `0.1.x` | Yes |
| `< 0.1.0` | No |

---

## Reporting a Vulnerability

If you discover a security vulnerability in PebblePost, please do not open a public GitHub issue. Instead, report it privately:

1. **Email**: Send details to **lovanbangbox9@gmail.com** with the subject line `[SECURITY] PebblePost Vulnerability Report`.
2. **Details to Include**:
   - Description of the vulnerability and attack vector.
   - Step-by-step reproduction instructions or proof-of-concept (PoC).
   - Affected version(s) and operating system environment.
3. **Response Timeline**:
   - We will acknowledge receipt of your vulnerability report within **48 hours**.
   - We will provide regular updates on our progress toward a fix.
   - Once a security patch is released, we will publicly credit the reporter (unless you request anonymity).
