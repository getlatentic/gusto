//! Signing in through the person's browser. The app keeps a secret and gives the browser only its hash; when the person has signed in
//! there, the browser comes back with a ticket by a `latentic://` link, and the ticket with the secret is traded for a session in this
//! app's own window. A ticket without the secret is of no use to a program that saw the link.

use crate::{config, window};
use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use sha2::{Digest, Sha256};
use std::sync::Mutex;
use tauri::{AppHandle, Manager};
use tauri_plugin_opener::OpenerExt;

/// The secret of the sign-in that is waiting for the person's browser.
#[derive(Default)]
pub struct Pending(Mutex<Option<String>>);

fn secret() -> Option<String> {
    let mut bytes = [0u8; 32];
    getrandom::fill(&mut bytes).ok()?;
    Some(URL_SAFE_NO_PAD.encode(bytes))
}

fn challenge(secret: &str) -> String {
    URL_SAFE_NO_PAD.encode(Sha256::digest(secret.as_bytes()))
}

/// Opens the person's browser at the server's sign-in, which brings them back to this app.
pub fn begin(app: &AppHandle) {
    let (Some(server), Some(secret)) = (config::load(app).server, secret()) else { return };
    let address = format!("{server}/auth/desktop/start?challenge={}", challenge(&secret));
    *app.state::<Pending>().0.lock().unwrap() = Some(secret);
    let _ = app.opener().open_url(address, None::<&str>);
}

/// Takes the ticket the browser came back with to the server, with the secret, which signs this window in.
pub fn complete(app: &AppHandle, ticket: &str) {
    let secret = app.state::<Pending>().0.lock().unwrap().take();
    let (Some(server), Some(secret)) = (config::load(app).server, secret) else { return };
    window::go(app, &format!("{server}/auth/desktop/redeem?ticket={ticket}&secret={secret}"));
}

#[cfg(test)]
mod tests {
    use super::challenge;

    #[test]
    fn the_challenge_is_the_hash_the_server_checks() {
        // The server's test makes the same value with Go's SHA-256 and unpadded base64url.
        assert_eq!(challenge("secret"), "K7gNU3sdo-OL0wNhqoVWhr3g6s1xYv72ol_pe_Unols");
        assert_eq!(challenge("secret").len(), 43);
    }
}
