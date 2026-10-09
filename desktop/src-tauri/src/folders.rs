//! Choosing a folder on this computer to work in. The page asks by going to an address of its own, which the window turns into the
//! system's folder chooser; the folder is given to the helper to keep the same as the agent's folder in the cloud, and the page is told
//! the key and the name, which are all the server ever learns of it.

use crate::{helper, window};
use sha2::{Digest, Sha256};
use std::path::Path;
use tauri::AppHandle;
use tauri_plugin_dialog::DialogExt;

/// What the server holds of a folder: the first sixteen hexadecimal digits of a hash of its path.
pub fn key_of(path: &str) -> String {
    Sha256::digest(path.as_bytes()).iter().take(8).map(|byte| format!("{byte:02x}")).collect()
}

fn name_of(path: &str) -> String {
    Path::new(path).file_name().map(|name| name.to_string_lossy().into_owned()).unwrap_or_else(|| "Folder".into())
}

/// Asks the person for a folder, and tells the page which one they chose.
pub fn pick(app: &AppHandle) {
    let app = app.clone();
    app.clone().dialog().file().set_title("Choose a folder to work in").pick_folder(move |chosen| {
        let Some(path) = chosen.and_then(|path| path.into_path().ok()) else { return };
        let path = path.to_string_lossy().into_owned();
        let key = key_of(&path);
        let name = name_of(&path);
        match helper::keep_folder(&app, &key, &path) {
            Ok(()) => window::tell_page(&app, &format!("window.latenticFolder && window.latenticFolder({})", serde_json::json!({ "key": key, "name": name }))),
            Err(why) => window::tell_page(&app, &format!("alert({})", serde_json::json!(format!("That folder cannot be used: {why}")))),
        }
    });
}

#[cfg(test)]
mod tests {
    use super::{key_of, name_of};

    #[test]
    fn a_folders_key_is_sixteen_digits_of_a_hash_of_its_path() {
        let key = key_of("/Users/somebody/work/thesis");
        assert_eq!(key.len(), 16);
        assert!(key.chars().all(|c| c.is_ascii_hexdigit() && !c.is_ascii_uppercase()));
        assert_ne!(key, key_of("/Users/somebody/work/other"));
        assert_eq!(key, key_of("/Users/somebody/work/thesis"));
    }

    #[test]
    fn a_folder_is_named_by_its_last_part() {
        assert_eq!(name_of("/Users/somebody/work/thesis"), "thesis");
        assert_eq!(name_of("/"), "Folder");
    }
}
