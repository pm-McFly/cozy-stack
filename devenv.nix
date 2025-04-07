{ pkgs, lib, config, inputs, ... }:
let
  couch = rec {
    user = "cozy";
    pass = user;
  };
  cozy = rec {
    admin_pass = "cozy";
    conf_folder_sh = "$DEVENV_ROOT";
    conf_file_sh = "${conf_folder_sh}/conf.yaml";
    storage_folder_sh = "${conf_folder_sh}/storage";
    admin_passphrase_file_sh = "${conf_folder_sh}/cozy-admin-passphrase";
    vault_credentials_files_sh = "${conf_folder_sh}/vault_credentials";
    vault_credentials_encryptor_key_file_sh = "${vault_credentials_files_sh}.enc";
    vault_credentials_decryptor_key_file_sh = "${vault_credentials_files_sh}.dec";
  };
  git = {
    message_file = ".devenv/git-message";
  };
in
{
  # todo: make stack port configurable

  packages = [
    pkgs.ghostscript
    pkgs.librsvg
    pkgs.lato
    pkgs.imagemagick
    pkgs.lato

    pkgs.golangci-lint
  ];

  services.couchdb.enable = true;
  services.couchdb.settings.couchdb.single_node = true;
  services.couchdb.settings.admins."${couch.user}" = "${couch.pass}";

  services.mailhog.enable = true;
  # services.mailhog.smtpListenAddress = "localhost:587"; # default from cozy.example.yaml

  languages.go.enable = true;
  languages.go.package = pkgs.go_1_23; # todo: should be 24 but not in nixpkgs stable yet

  languages.javascript.enable = true;
  languages.javascript.npm.enable = true;
  languages.javascript.npm.install.enable = true;
  languages.javascript.directory = "./scripts";

  scripts._dc_lint_go.exec = ''
    # based on Makefile target lint (doing this to avoid the curl | sh)
    golangci-lint run --verbose
  '';
  scripts._dc_lint_js.exec = ''
    cd "$DEVENV_ROOT"
    make jslint
  '';
  scripts._dc_ensure_cozy-stack_dev_required_files.exec = ''
    set -euo pipefail
    cd "$DEVENV_ROOT"
    make install
    if ! which "cozy-stack" > /dev/null; then
      echo "Building cozy-stack"
      go build
    fi
    EXAMPLE_CONF="$DEVENV_ROOT/cozy.example.yaml"
    [[ -f "${cozy.conf_file_sh}" ]] || (
      echo "Missing ${cozy.conf_file_sh} : copying from $EXAMPLE_CONF"
      cp "$EXAMPLE_CONF" "${cozy.conf_file_sh}"
    )
    [[ -f "${cozy.admin_passphrase_file_sh}" ]] || (
      echo "Missing ${cozy.admin_passphrase_file_sh} : setting it"
      COZY_ADMIN_PASSPHRASE="${cozy.admin_pass}" cozy-stack config passwd "${cozy.admin_passphrase_file_sh}"
    )
    (
      [[ -f "${cozy.vault_credentials_encryptor_key_file_sh}" ]] ||
      [[ -f "${cozy.vault_credentials_decryptor_key_file_sh}" ]]
    ) || (
      echo "Missing ${cozy.vault_credentials_files_sh} pair : creating it"
      cozy-stack config gen-keys "${cozy.vault_credentials_files_sh}"
    )
  '';

  processes = {
    # cozy-stack-serve.exec = "make run";
    # --log-level debug
    # 
    cozy-stack-serve.exec = ''
      _dc_ensure_cozy-stack_dev_required_files && \
      go run . serve \
        --mailhog \
        --couchdb-url 'http://${couch.user}:${couch.pass}@localhost:5984' \
        "--fs-url=file://localhost${cozy.storage_folder_sh}" \
        --konnectors-cmd ''${DEVENV_ROOT}/scripts/konnector-dev-run.sh \
        --config "${cozy.conf_file_sh}" \
        --vault-decryptor-key "${cozy.vault_credentials_decryptor_key_file_sh}" \
        --vault-encryptor-key "${cozy.vault_credentials_encryptor_key_file_sh}"
    '';
  };

  files."${git.message_file}".text = ''
    # feat/fix/docs/style/refactor/test/chore(optional scope): Subject

    # optional body

    # BREAKING CHANGE:
    # Resolves: #123
    # See also: #456, #789
    #
    # feat: a new feature
    # fix: a bug fix
    # docs: changes to documentation
    # style: formatting, missing semi colons, etc; no code change
    # refactor: refactoring production code; no behavior change
    # test: adding tests, refactoring test; no production code change
    # chore: updating build tasks, package manager configs, etc; no production code change
  '';

  git-hooks.hooks = {
    go-lint = {
      enable = true;
      name = "go-lint";
      entry = "_dc_lint_go";
      files = "\\.(go)$";
    };
    js-lint = {
      enable = true;
      name = "js-lint";
      entry = "_dc_lint_js";
      files = "\\.(js|ts)x?$";
    };
  };

  enterShell = ''
    set -euo pipefail
    export PATH="$(go env GOPATH)/bin:$PATH"
    _dc_ensure_cozy-stack_dev_required_files
    git config commit.template "$DEVENV_ROOT/${git.message_file}"

    echo 'Use `devenv up` first !'
    echo "🛋️ fauxton    at http://127.0.0.1:5984/_utils (user: ${couch.user}, pass: ${couch.pass})"
    echo "☁️ cozy       at http://cozy.localhost:8080/ (default pass: cozy)"
    echo "🛠️ cozy admin at http://127.0.0.1:6060/ (pass: ${cozy.admin_pass})"
    echo "✉️ mailhog    at http://127.0.0.1:8025/"
  '';

  #### subjective ####
  difftastic.enable = true;
}