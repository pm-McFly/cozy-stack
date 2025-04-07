[Table of contents](README.md#table-of-contents)

# How to install Cozy-stack?

## Dependencies

- A reverse-proxy (nginx, caddy, haproxy, etc.)
- A SMTP server
- CouchDB 3
- Git
- Image Magick (and the Lato font, ghostscript et rsvg-convert)

To install CouchDB 3 through Docker, take a look at our
[Docker specific documentation](docker.md).

**Note:** to generate thumbnails for heic/heif images, the version 6.9+ of
Image Magick is required.

## Install for self-hosting

We have started to write documentation on how to install cozy on your own
server. We have [guides for
self hosting](https://docs.cozy.io/en/tutorials/selfhosting/), either on
Debian with precompiled binary packages of from sources on Ubuntu.
Don't hesitate to [report issues](https://github.com/cozy/cozy.github.io/issues/new) with them.
It will help us improve documentation.

## Install for development / local tests

Using [`devenv`](https://devenv.sh/)? refer to [Load devenv](INSTALL.md#load-devenv).

### Install the binary

You can either download the binary or compile it.

#### Download an official release

You can download a `cozy-stack` binary from our official releases:
https://github.com/cozy/cozy-stack/releases. It is a just a single executable
file (choose the one for your platform). Rename it to cozy-stack, give it the
executable bit (`chmod +x cozy-stack`) and put it in your `$PATH`.
`cozy-stack version` should show you the version if every thing is right.

#### Compile the binary using `go`

You can compile a `cozy-stack` from the source.
First, you need to [install go](https://golang.org/doc/install), version >=
1.21. With `go` installed and configured, you can run the following commands:

```
git clone git@github.com:cozy/cozy-stack.git
cd cozy-stack
make
```

This will fetch the sources and build a binary in `$GOPATH/bin/cozy-stack`.

Don't forget to add your `$GOPATH/bin` to your `$PATH` at the end of your `*rc` file so
that you can execute the binary without entering its full path.

```
export PATH="$(go env GOPATH)/bin:$PATH"
```

#### Troubleshooting

Check if you don't have an alias "go" configurated in your `*rc` file.

#### Server your local cozy

You can configure your `cozy-stack` using a configuration file or different
comand line arguments. Assuming CouchDB is installed and running on default port
`5984`, you can start the server:

```bash
cozy-stack serve
```

### Load devenv

Devenv allows you to automatically load a ready-to-use environment containing all required tools and services.
It automatically loads when entering project folder and keeps everything up-to-date or on the correct version for you.

> Please, make sure to use version >=1.4.1. Follow the guide:
>
> - [Installation](https://devenv.sh/getting-started/#installation)
> - [Updating](https://devenv.sh/getting-started/#updating)

Once installed, your environment should load-up directly when entering the project folder.

```bash
### Example output: ###

direnv: loading /path/to/cozy-stack/.envrc
direnv: using devenv
• Building shell ...
• Using Cachix: devenv
✔ Building shell in 207ms
Running tasks     devenv:enterShell
Succeeded         devenv:git-hooks:install 32ms
Succeeded         devenv:files             20ms
Succeeded         devenv:enterShell        19ms
3 Succeeded                                52.57ms

Use `devenv up` first !
🛋️ fauxton    at http://127.0.0.1:5984/_utils (user: cozy, pass: cozy)
☁️ cozy       at http://cozy.localhost:8080/ (default pass: cozy)
🛠️ cozy admin at http://127.0.0.1:6060/ (pass: cozy)
✉️ mailhog    at http://127.0.0.1:8025/
direnv: export +AR +AS +CC +CONFIG_SHELL +CXX +DEVENV_DIRENVRC_ROLLING_UPGRADE +DEVENV_DIRENVRC_VERSION +DEVENV_DOTFILE +DEVENV_PROFILE +DEVENV_ROOT +DEVENV_RUNTIME +DEVENV_STATE +DEVENV_TASKS +ERL_FLAGS +GETTEXTDATADIRS_FOR_BUILD +GIT_EXTERNAL_DIFF +GOPATH +GOROOT +GOTOOLCHAIN +GOTOOLDIR +IN_NIX_SHELL +LD +NIX_BINTOOLS +NIX_BINTOOLS_WRAPPER_TARGET_HOST_<host_arch_and_kernel> +NIX_CC +NIX_CC_WRAPPER_TARGET_HOST_<host_arch_and_kernel> +NIX_CFLAGS_COMPILE +NIX_ENFORCE_NO_NATIVE +NIX_HARDENING_ENABLE +NIX_LDFLAGS +NIX_PKG_CONFIG_WRAPPER_TARGET_HOST_<host_arch_and_kernel> +NIX_STORE +NM +NODE_PATH +OBJCOPY +OBJDUMP +PC_CONFIG_FILES +PC_SOCKET_PATH +PKG_CONFIG +PKG_CONFIG_PATH +RANLIB +READELF +SIZE +SOURCE_DATE_EPOCH +STRINGS +STRIP +cmakeFlags +configureFlags +hardeningDisable +mesonFlags +name +system ~GDK_PIXBUF_MODULE_FILE ~PATH ~XDG_DATA_DIRS
```

To start a local instance with all associated services running, use:

```bash
devenv up
```

Your local server is now started! *(to kill it, close your terminal or use \<F10\>)*

***Please, open a new terminal to follow the rest of the documentation.***

### Add an instance for testing

And then create an instance for development:

```bash
make instance
```

The cozy-stack server listens on http://cozy.localhost:8080/ by default. See
`cozy-stack --help` for more informations.

The above command will create an instance on http://cozy.localhost:8080/ with the
passphrase `cozy`. By default this will create a `storage/` entry in your current directory, containing all your instances by their URL. An instance "cozy.localhost:8080" will have its stored files in `storage/cozy.localhost:8080/`. Installed apps will be found in the `.cozy_apps/` directory of each instance.

Make sure the full stack is up with:

```bash
curl -H 'Accept: application/json' 'http://cozy.localhost:8080/status/'
```

You can then remove your test instance:

```bash
cozy-stack instances rm cozy.localhost:8080
```
