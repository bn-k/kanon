use zed_extension_api::{self as zed, settings::LspSettings, LanguageServerId, Result};

struct KanonExtension;

impl zed::Extension for KanonExtension {
    fn new() -> Self {
        KanonExtension
    }

    fn language_server_command(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        // The path set in Zed's settings, under lsp.kan.binary.path, wins over the kan found on the PATH.
        let binary = LspSettings::for_worktree(language_server_id.as_ref(), worktree)
            .ok()
            .and_then(|settings| settings.binary);
        let command = match binary.as_ref().and_then(|binary| binary.path.clone()) {
            Some(path) => path,
            None => worktree.which("kan").ok_or_else(|| {
                "kan is not on the PATH: install it with `go install github.com/bn-k/kanon/cmd/kan@latest`, \
                 or set lsp.kan.binary.path in Zed's settings"
                    .to_string()
            })?,
        };
        let args = binary
            .and_then(|binary| binary.arguments)
            .unwrap_or_else(|| vec!["lsp".to_string()]);
        Ok(zed::Command {
            command,
            args,
            env: worktree.shell_env(),
        })
    }
}

zed::register_extension!(KanonExtension);
