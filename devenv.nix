{ pkgs, lib, config, inputs, ... }:
let
  couch = rec {
    user = "cozy";
    pass = user;
  };
  cozy = rec {
    port = 8080;
    admin_port = 6060;
    admin_pass = "cozy";
    conf_folder_sh = "$DEVENV_ROOT";
    conf_file_sh = "${conf_folder_sh}/conf.yaml";
    vault_credentials_files_sh = "${conf_folder_sh}/vault_credentials";

    instance = rec {
      domain = "cozy.localhost";
      name = "Claude";
      pass = "cozy";
      email = "claude@cozy.localhost";
      apps = "home,store,drive,photos,settings,contacts,notes,passwords";
      locale = "fr";
      context = "dev";
    };
  };
  git = {
    message_file = ".devenv/git-message";
  };

  # These values shouldn't be manually set
  computed = {
    couch = {
      url = "http://${couch.user}:${couch.pass}@localhost:5984";
    };
    cozy = {
      storage_folder_sh = "${cozy.conf_folder_sh}/storage";
      admin_passphrase_file_sh = "${cozy.conf_folder_sh}/cozy-admin-passphrase";
      vault_credentials_encryptor_key_file_sh = "${cozy.vault_credentials_files_sh}.enc";
      vault_credentials_decryptor_key_file_sh = "${cozy.vault_credentials_files_sh}.dec";
      completion_base_path_sh = "$DEVENV_ROOT/scripts/completion/cozy-stack.";

      instance = {
        domain = "${cozy.instance.domain}:${toString cozy.port}";
      };
    };
  };
  pkgs-unstable = import inputs.nixpkgs-unstable { system = pkgs.stdenv.system; };
in {
  # todo: make stack port configurable
  # todo: add conf file to gitignore
  # todo: ruby stuff in tests/system


  packages = [
    pkgs.ghostscript
    pkgs.librsvg
    # pkgs.imagemagick
    pkgs.imagemagick6 # insecure, has exception in devenv.yaml
    pkgs.lato

    pkgs.golangci-lint # DCS_GOLANGCI_LINT_METHOD=manual

    pkgs.gnused # for sh http client test # TODO NONONO
  ];

  services.redis.enable = true;

  services.couchdb.enable = true;
  services.couchdb.settings.couchdb.single_node = true;
  services.couchdb.settings.admins."${couch.user}" = "${couch.pass}";
  env.COZY_COUCHDB_URL = "${computed.couch.url}";

  services.mailhog.enable = true;
  # services.mailhog.smtpListenAddress = "localhost:587"; # default from cozy.example.yaml

  languages.go.enable = true;
  languages.go.package = pkgs.go_1_23; # todo: should be 24 but not in nixpkgs stable yet
  # languages.go.package = pkgs-unstable.go_1_24;

  languages.javascript.enable = true;
  languages.javascript.npm.enable = true;
  languages.javascript.npm.install.enable = true;
  languages.javascript.directory = "./scripts";

  languages.ruby.enable = true;

  env.COZY_ADMIN_PASSWORD = cozy.admin_pass;

  # Scripts

  #   The convention for script names is :
  #     - `__dcs*`: scripts used exclusively by other scripts
  #     - `_dcs*`: scripts used internally by this devenv (eg. git hooks), or manual but technical related to it
  #     - `dcs*`: scripts used by the user
  #   *dcs = devenv cozy stack*

  ## Common header

  ### include with: . "$(which __dcs_common)"
  scripts.__dcs_common.exec = ''
    set -euo pipefail
    info() { echo "$@" >&2; }
    fail() { info "$@" ; exit 1; }
    ENTRY_CMD="''${0##*/}"
  '';
  ### with cd into DEVENV_ROOT, include with: . "$(which __dcs_common)"
  scripts.__dcs_common_cd_root.exec = ''
    . "$(which __dcs_common)"
    cd "$DEVENV_ROOT"
  '';

  ## Git stuff

  ### Hooks
  scripts._dcs_lint_go_golangci-lint_version.exec = ''
    . "$(which __dcs_common_cd_root)"

    # used by DCS_GOLANGCI_LINT_METHOD=docker|docker-with-cache
    # echo Version in Makefile:
    # grep 'golangci/golangci-lint' Makefile | sed -E 's/.*(v[0-9.]+)$/\1/'
    # echo Version used in Gitlab CI Curl:
    # #  - see https://github.com/golangci/golangci-lint-action/tree/v5/
    # #  - see https://github.com/cozy/cozy-stack/actions/runs/14388162975/job/40348569319#step:4:19
    # curl -o- --silent 'https://raw.githubusercontent.com/golangci/golangci-lint/master/assets/github-action-config.json' | jq -r '.MinorVersionToConfig.latest.TargetVersion'

    # echo Version in ${pkgs.golangci-lint}:
    # "${pkgs.golangci-lint}/bin/golangci-lint" --version | sed -E 's/.* has version ([0-9a-zA-Z.-]+).*/v\1/'
    # # Version used in CI as of 20250413
    echo "v1.64.8"
  '';
  scripts.dcs_lint_go.exec = ''
    . "$(which __dcs_common_cd_root)"
    # # DCS_GOLANGCI_LINT_METHOD=manual
    "${pkgs.golangci-lint}/bin/golangci-lint" run --verbose \
      --exclude-files pkg/i18n/i18n.go \
      "$@"
    GOLANGCI_VERSION="$(_dcs_lint_go_golangci-lint_version)"
    info "Using golangci-lint version: $GOLANGCI_VERSION"
    # # DCS_GOLANGCI_LINT_METHOD=docker
    # docker run -t --rm -v $(pwd):/app -w /app "golangci/golangci-lint:$GOLANGCI_VERSION" golangci-lint run
    # # DCS_GOLANGCI_LINT_METHOD=docker-with-cache
    # docker run --rm -t -v $(pwd):/app -w /app \
    #   --user $(id -u):$(id -g) \
    #   -v $(go env GOCACHE):/.cache/go-build -e GOCACHE=/.cache/go-build \
    #   -v $(go env GOMODCACHE):/.cache/mod -e GOMODCACHE=/.cache/mod \
    #   -v $DEVENV_ROOT/.devenv/docker-golangci-lint-cache/golangci-lint:/.cache/golangci-lint \
    #   -e GOLANGCI_LINT_CACHE=/.cache/golangci-lint \
    #   "golangci/golangci-lint:$GOLANGCI_VERSION" golangci-lint run
  '';
  scripts.dcs_lint_js.exec = ''
    cd "$DEVENV_ROOT"
    make jslint
  '';
  git-hooks.hooks = {
    golangci-lint.enable = true; # DCS_GOLANGCI_LINT_METHOD=devenv # this method does not make manual running easy

    # go-lint = {  # DCS_GOLANGCI_LINT_METHOD=manual|docker|docker-with-cache
    #   enable = true;
    #   name = "go-lint";
    #   entry = "dcs_lint_go";
    #   files = "\\.(go)$";
    # };
    js-lint = {
      enable = true;
      name = "js-lint";
      entry = "dcs_lint_js";
      files = "\\.(js|ts)x?$";
    };
  };

  ### Commit message template
  scripts._dcs_ensure_git_message_template.exec = ''
    git config commit.template "$DEVENV_ROOT/${git.message_file}"
  '';
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

  ## Auto-generated required files for runtime
  scripts._dcs_delete_cozy-stack_dev_required_files.exec = ''
    . "$(which __dcs_common_cd_root)"
    info "Deleting cozy-stack dev required files, to recreate:"
    info "  - re-enter the devenv (eg. cd out and back in)"
    info "  - run _dcs_ensure_cozy-stack_dev_required_files"
    set +e
    GO_BIN="$(go env GOPATH)/bin"
    set -x
    rm -rf \
      "${cozy.conf_file_sh}" \
      "${computed.cozy.admin_passphrase_file_sh}" \
      "${computed.cozy.vault_credentials_encryptor_key_file_sh}" \
      "${computed.cozy.vault_credentials_decryptor_key_file_sh}" \
      "${computed.cozy.storage_folder_sh}" \
      "''${GO_BIN}/cozy-stack" \
      || true
    go clean
  '';
  scripts._dcs_ensure_cozy-stack_dev_required_files.exec = ''
    . "$(which __dcs_common_cd_root)"
    make install
    EXAMPLE_CONF="$DEVENV_ROOT/cozy.example.yaml"
    [[ -f "${cozy.conf_file_sh}" ]] || (
      info "Missing ${cozy.conf_file_sh} : copying from $EXAMPLE_CONF"
      cp "$EXAMPLE_CONF" "${cozy.conf_file_sh}"
    )
    [[ -f "${computed.cozy.admin_passphrase_file_sh}" ]] || (
      info "Missing ${computed.cozy.admin_passphrase_file_sh} : setting it"
      COZY_ADMIN_PASSPHRASE="${cozy.admin_pass}" go run . config passwd "${computed.cozy.admin_passphrase_file_sh}"
    )
    (
      [[ -f "${computed.cozy.vault_credentials_encryptor_key_file_sh}" ]] ||
      [[ -f "${computed.cozy.vault_credentials_decryptor_key_file_sh}" ]]
    ) || (
      info "Missing ${cozy.vault_credentials_files_sh} pair : creating it"
      go run . config gen-keys "${cozy.vault_credentials_files_sh}"
    )
  '';

  ## Quality of life
  scripts.dcs_test_go.exec = ''
    . "$(which __dcs_common)"
    usage() {
      [[ "$#" == 0 ]] || info "Error: $@"
      fail "Usage: $ENTRY_CMD <path> <test-name or 'all'>"
    }
    go_test() {
      info "$" go test "$@"
      go test "$@"
    }
    [[ -n "''${1:-}" ]] || usage "missing arguments"
    [[ -d "''${1:-}" ]] || usage "$1 is not a directory"
    if [[ -n "''${2:-}" ]]; then
      if [[ "$2" == "all" ]]; then
        go_test "$1" "''${@:3}"
      else
        go_test "$1" -run "''${@:2}"
      fi
    else
      info "Error: Missing test name. Use 'all' to run all tests."
      info "Tests in $1:"
      # go test "$1" -list=.
      for f in "$1"/*_test.go; do
        echo "- $f:"
        node - "$f" <<'EOF'
          const indent = "    ";
          const stack = [], warnings = [], tests = [];
          require('fs').readFileSync(process.argv[2], "utf8").trim().split("\n").forEach((line, lineNo) => {
            const m = line.match(/^(?<indent>\s*)((func (?<func>Test[a-zA-Z0-9_-]+)\()|(t.Run\((("(?<run>[^"]+)")|(?<run_other>[^)]+))))/);
            if (!m) return;
            const { indent, func, run, run_other } = m.groups;
            if (run_other)
              return warnings.push(["⚠ Warning: didn't understand line", lineNo + 1, ":", JSON.stringify(line)])
            const depth = (func ? 0 : 1) + indent.length;
            while (stack.length > 0 && stack[stack.length - 1].depth >= depth)
              stack.pop();
            const name = func ? func.replace(/^Test/, "") : run;
            const display = name; //.replace(/([A-Z]+)/g, " $1").replace(/([A-Z][a-z])/g, " $1").trim();
            stack.push({ depth, name });
            tests.push({ depth, name: stack.map(({ name }) => name).join("/"), display });
          });
          if (!tests.some(({ depth }) => depth == 1))
            tests.forEach(test => test.depth = test.depth > 1 ? test.depth - 1 : test.depth);
          tests.forEach(test => test.display = indent.repeat(test.depth) + test.display);
          let displayMaxLen = tests.reduce((max, { display }) => Math.max(max, display.length), 0);
          displayMaxLen = displayMaxLen + (displayMaxLen % 4 ? 4 - (displayMaxLen % 4) : 0);
          warnings.forEach(a => console.warn(indent, ...a));
          tests.forEach(test => console.log(indent, test.display.padEnd(displayMaxLen), indent, test.name));
    EOF
        # grep -E "^\s*(func Test[a-zA-Z0-9_-]+\()|(t.Run\()" "$f" # | sed -E 's/^(\s*)t.Run\(\"([^\"]+)\".*/  \1\2/'
      done
      exit 1
    fi
  '';
  scripts.dcs_cozy-stack.exec = ''
    go run . \
      --port "${toString cozy.port}" \
      --admin-port "${toString cozy.admin_port}" \
      "$@"
  '';
  scripts.dcs_cozy-stack_instance_add.exec = ''
    . "$(which __dcs_common)"
  	if dcs_cozy-stack instances show "${computed.cozy.instance.domain}" > /dev/null 2>&1 ; then
      info "⚠ Warning: ${computed.cozy.instance.domain} already exists, use:"
      info "    dcs_cozy-stack instances rm \"${computed.cozy.instance.domain}\""
      info "    or maybe: rm -rf .devenv/state/couchdb"
    else
      dcs_cozy-stack instances add \
        --dev \
        "${computed.cozy.instance.domain}" \
        --passphrase "${cozy.instance.pass}" \
        --apps "${cozy.instance.apps}" \
        --email "${cozy.instance.email}" \
        --locale "${cozy.instance.locale}" \
        --public-name "${cozy.instance.name}" \
        --context-name "${cozy.instance.context}"
    fi
    if ! grep "${cozy.instance.domain}" /etc/hosts >/dev/null ; then
      info "⚠ Warning: domain isn't in hosts file, run:"
      fail "    (echo ; echo \"127.0.0.1 ${cozy.instance.domain}\") | sudo tee -a /etc/hosts"
    fi
  '';
  scripts.dcs_print_completion_path.exec = ''
    . "$(which __dcs_common)"
    COZY_COMPLETIONS_FILE="${computed.cozy.completion_base_path_sh}$(basename "$SHELL")"
    if [[ -f "$COZY_COMPLETIONS_FILE" ]]; then
      # echo "$COZY_COMPLETIONS_FILE"
      TEMP_COMPLETIONS_FILE="''${DEVENV_ROOT}/.devenv/completions.$(basename "$SHELL")"
      (
        cat "$COZY_COMPLETIONS_FILE"
        echo
        echo "# And now the encore for dcs_cozy-stack"
        cat "$COZY_COMPLETIONS_FILE" | sed 's/cozy-stack/dcs_cozy-stack/g'
      ) > "$TEMP_COMPLETIONS_FILE"
      echo "$TEMP_COMPLETIONS_FILE"
    else
      fail "Unknown shell '$SHELL' (looked for $COZY_COMPLETIONS_FILE)"
    fi
  '';

  processes = {
    # --log-level debug
    # --disable-csp
    cozy-stack-serve.exec = ''
      _dcs_ensure_cozy-stack_dev_required_files && \
      dcs_cozy-stack serve \
        --dev \
        --disable-csp \
        --mailhog \
        --couchdb-url "${computed.couch.url}" \
        "--fs-url=file://localhost${computed.cozy.storage_folder_sh}" \
        --konnectors-cmd ''${DEVENV_ROOT}/scripts/konnector-dev-run.sh \
        --config "${cozy.conf_file_sh}" \
        --vault-decryptor-key "${computed.cozy.vault_credentials_decryptor_key_file_sh}" \
        --vault-encryptor-key "${computed.cozy.vault_credentials_encryptor_key_file_sh}" \
        "$@"
    '';
  };

  enterShell = ''
    . "$(which __dcs_common)"
    export PATH="$(go env GOPATH)/bin:$PATH"
    _dcs_ensure_cozy-stack_dev_required_files || echo "⚠ Error building or generating keys"
    _dcs_ensure_git_message_template
    if [[ "''${COMPLETIONS_TO_SOURCE_FROM_DEVENV_TO_WHATEVER_SHELLRC_SUPPORTED:-}" == "1" ]]; then
      export COMPLETIONS_TO_SOURCE_FROM_DEVENV_TO_WHATEVER_SHELLRC="$(dcs_print_completion_path --generic)"
    else
      info "🛈 in your shell-rc file, add this to support completion: "
    fi
    info '🛈 shell completion with: . "$(dcs_print_completion_path)", commands start with `dcs_*`'
    info '🛈 use `devenv up` first !'
    info "🛋️ fauxton    at http://127.0.0.1:5984/_utils (user: ${couch.user}, pass: ${couch.pass})"
    info "☁️ cozy       at http://${computed.cozy.instance.domain} (pass: ${cozy.instance.pass}, create with dcs_cozy-stack_instance_add)"
    info "🛠️ cozy admin at http://127.0.0.1:${toString cozy.admin_port}/ (pass: ${cozy.admin_pass})"
    info "✉️ mailhog    at http://127.0.0.1:8025/"
    # echo ${pkgs.lato}
    export _ERIC_SPECIAL_DIR_ENV_HOOK_STATE="dcs_print_completion_path"
  '';

  #### subjective ####
  difftastic.enable = true;
}
