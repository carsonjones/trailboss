use std::collections::BTreeMap;
use zellij_tile::prelude::*;
use zellij_tile::shim::unblock_cli_pipe_input;
use serde::Deserialize;

#[derive(Default)]
struct State {
    ready: bool,
}

#[derive(Deserialize)]
struct Msg {
    action: String,
    name: String,
    script: String,
}

impl ZellijPlugin for State {
    fn load(&mut self, _config: BTreeMap<String, String>) {
        request_permission(&[
            PermissionType::ReadApplicationState,
            PermissionType::ChangeApplicationState,
        ]);
        subscribe(&[EventType::PermissionRequestResult]);
    }

    fn update(&mut self, event: Event) -> bool {
        if let Event::PermissionRequestResult(PermissionStatus::Granted) = event {
            self.ready = true;
            set_selectable(false);
        }
        false
    }

    fn render(&mut self, _rows: usize, _cols: usize) {}

    fn pipe(&mut self, msg: PipeMessage) -> bool {
        unblock_cli_pipe_input(&msg.name);

        if !self.ready {
            return false;
        }

        let payload = msg.payload.as_deref().unwrap_or("");
        let parsed = match serde_json::from_str::<Msg>(payload) {
            Ok(p) => p,
            Err(e) => {
                eprintln!("trailboss: bad payload: {e}\n  payload: {payload}");
                return false;
            }
        };

        if parsed.action != "new_tab" {
            return false;
        }

        let layout = format!(
            r#"layout {{
    tab name="{name}" focus=true {{
        pane command="bash" {{
            args "-c" "{script}"
        }}
    }}
}}"#,
            name = parsed.name.replace('"', "\\\""),
            script = parsed.script.replace('"', "\\\""),
        );

        new_tabs_with_layout(&layout);
        false
    }
}

register_plugin!(State);
