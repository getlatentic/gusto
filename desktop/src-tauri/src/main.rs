// The window is not a console program on Windows: it is started at login and must open no black window.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    latentic_agent_cloud_lib::run()
}
