/// Replace each configured secret with the marker used by PebblePost logs and reports.
///
/// Values shorter than three bytes are left alone to avoid redacting common text.
pub fn mask_secrets(text: &str, secrets: &[String]) -> String {
    secrets
        .iter()
        .filter(|secret| secret.len() >= 3)
        .fold(text.to_owned(), |text, secret| text.replace(secret, "***"))
}
