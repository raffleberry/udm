# udm development tasks.
#
# The installer must run with internal/host as the working directory: it resolves
# both the host entrypoint and resources/udm.png relative to the process cwd.

set shell := ["bash", "-c"]

default:
    @just --list

# Compile the host and register it with Firefox, plus the icon in hicolor.
install-host:
    cd internal/host && go run ./cmd/install

# Run the app. This is the supported dev path: it picks aria-free defaults, logs
# to the config dir, and finds resources/ relative to the repo root.
run:
    go run ./cmd/udm

# Everything that must pass before a change lands.
check: tidy vet test build

# Compile every package and command, including the build-tagged tray.
build:
    go build ./...
    GOOS=windows go build ./...

tidy:
    go mod tidy

vet:
    go vet ./...

test:
    go test ./...

# Run the app with the GTK debug chatter, for window and tray problems.
debug:
    TK_DEBUG=1 go run ./cmd/udm

# Print the port the extension and the native host agree on.
port:
    @go run ./cmd/udm -print-port
