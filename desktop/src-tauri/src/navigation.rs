//! Where the window may go. It shows the server's pages. A page of another site opens in the person's browser, and sign-in is done
//! there too: a provider may refuse a window that is not a browser, and the person's passkeys and sessions are in their browser.

use url::Url;

#[derive(Debug, PartialEq)]
pub enum Move {
    /// The window goes there.
    Allow,
    /// The window stays, and the person signs in through their browser.
    SignIn,
    /// The window stays, and the person chooses a folder of this computer to work in.
    PickFolder,
    /// The window goes to this address instead: the sign-in page that offers the browser.
    Instead(Url),
    /// The window stays, and the app signs it in to gusto on this computer again, with a new link.
    SignInHere,
    /// The window stays, and the person's browser opens it.
    Browser,
}

/// What to do about the window going to `to`, when it is showing `server`, which is gusto on this computer when `here`.
pub fn decide(server: &Url, to: &Url, here: bool) -> Move {
    if !matches!(to.scheme(), "http" | "https") || to.host_str() == Some("tauri.localhost") {
        return Move::Allow;
    }
    if to.origin() != server.origin() {
        return Move::Browser;
    }
    match to.path() {
        "/login" if here => Move::SignInHere,
        "/auth/desktop/begin" => Move::SignIn,
        "/app/folder" => Move::PickFolder,
        "/login" if !to.query_pairs().any(|(name, _)| name == "desktop") => {
            let mut offered = to.clone();
            offered.query_pairs_mut().append_pair("desktop", "1");
            Move::Instead(offered)
        }
        _ => Move::Allow,
    }
}

#[cfg(test)]
mod tests {
    use super::{decide, Move};
    use url::Url;

    fn go(to: &str) -> Move {
        decide(&Url::parse("https://agents.example.com").unwrap(), &Url::parse(to).unwrap(), false)
    }

    #[test]
    fn gusto_on_this_computer_is_signed_in_to_again_by_the_app() {
        let here = |to: &str| decide(&Url::parse("http://127.0.0.1:7140").unwrap(), &Url::parse(to).unwrap(), true);
        assert_eq!(here("http://127.0.0.1:7140/login"), Move::SignInHere);
        assert_eq!(here("http://127.0.0.1:7140/chat"), Move::Allow);
        assert_eq!(here("http://127.0.0.1:7141/model"), Move::Browser);
    }

    #[test]
    fn the_servers_pages_open_in_the_window() {
        assert_eq!(go("https://agents.example.com/chat"), Move::Allow);
        assert_eq!(go("https://agents.example.com/auth/desktop/redeem?ticket=t&secret=s"), Move::Allow);
        assert_eq!(go("tauri://localhost/index.html"), Move::Allow);
        assert_eq!(go("http://tauri.localhost/index.html"), Move::Allow);
        assert_eq!(go("about:blank"), Move::Allow);
    }

    #[test]
    fn another_site_opens_in_the_browser() {
        assert_eq!(go("https://accounts.google.com/o/oauth2/v2/auth"), Move::Browser);
        assert_eq!(go("https://agents.example.com.evil.example/chat"), Move::Browser);
        assert_eq!(go("http://agents.example.com/chat"), Move::Browser);
    }

    #[test]
    fn the_sign_in_page_offers_the_browser() {
        match go("https://agents.example.com/login?refused=no") {
            Move::Instead(page) => assert_eq!(page.as_str(), "https://agents.example.com/login?refused=no&desktop=1"),
            other => panic!("{other:?}"),
        }
        assert_eq!(go("https://agents.example.com/login?desktop=1"), Move::Allow);
        assert_eq!(go("https://agents.example.com/auth/desktop/begin"), Move::SignIn);
        assert_eq!(go("https://agents.example.com/app/folder"), Move::PickFolder);
    }
}
