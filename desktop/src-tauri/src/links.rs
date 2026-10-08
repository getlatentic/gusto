//! `latentic://` links: they open a page of the server the app is connected to, and never another address. A link that names the
//! address of a different server, or of no server, is ignored, so that a link in a message cannot take the window anywhere.

use url::Url;

/// The address to show for a link, or nothing when the link is not one of ours. A link is `latentic://open?path=/instances/1/threads/2`.
pub fn target(link: &str, server: &str) -> Option<String> {
    let link = Url::parse(link).ok()?;
    if link.scheme() != "latentic" || link.host_str() != Some("open") {
        return None;
    }
    let path = link.query_pairs().find(|(name, _)| name == "path").map(|(_, value)| value.into_owned())?;
    // A path begins with one slash. Two would be another host, a colon before the first slash another scheme, and a backslash is read as a slash.
    if !path.starts_with('/') || path.starts_with("//") || path.contains('\\') || path.contains('\n') {
        return None;
    }
    let base = Url::parse(server).ok()?;
    let joined = base.join(&path).ok()?;
    if joined.origin() != base.origin() {
        return None;
    }
    Some(joined.to_string())
}

#[cfg(test)]
mod tests {
    use super::target;

    const SERVER: &str = "https://agents.example.com";

    #[test]
    fn a_link_to_a_page_of_the_server_shows_it() {
        assert_eq!(
            target("latentic://open?path=/instances/i1/threads/t1", SERVER).as_deref(),
            Some("https://agents.example.com/instances/i1/threads/t1")
        );
        assert_eq!(target("latentic://open?path=%2Fjobs%2Fj9%3Fx%3D1", SERVER).as_deref(), Some("https://agents.example.com/jobs/j9?x=1"));
    }

    #[test]
    fn a_link_that_would_leave_the_server_or_is_not_ours_is_ignored() {
        for link in [
            "latentic://open?path=//evil.example/x",
            "latentic://open?path=https://evil.example/x",
            "latentic://open?path=evil.example",
            "latentic://open?path=/\\evil.example",
            "latentic://open?path=@evil.example/x",
            "latentic://open",
            "latentic://connect?server=https://evil.example",
            "https://agents.example.com/instances/1",
            "javascript:alert(1)",
            "not a url",
        ] {
            assert_eq!(target(link, SERVER), None, "{link}");
        }
    }

    #[test]
    fn a_port_is_part_of_the_server() {
        assert_eq!(target("latentic://open?path=/x", "http://localhost:8040").as_deref(), Some("http://localhost:8040/x"));
    }
}
