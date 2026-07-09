# QuickText

A lightweight, fast and minimal text editor for Linux, built with **Wails v2** (Go + Vue 3).

![QuickText](build/appicon.png)

## Features

- **Fast native editing** — Monaco Editor on the frontend, Go on the backend for instant disk I/O.
- **Tabs** — open and edit multiple files at once, with a dirty-state indicator per tab.
- **File tree sidebar** — browse folders and jump between files.
- **Built-in terminal** — an embedded PTY shell (`xterm.js`) running your default shell.
- **Safe close** — closing the window with unsaved changes prompts a *Save / Discard / Cancel* dialog instead of losing work.
- **Keyboard shortcuts**
  - `Ctrl + S` — save current file
  - `Ctrl + O` — open file
  - `Ctrl + F` — find (Monaco native search)
  - `Ctrl + Mouse Wheel` — zoom font size
- **Autosave** — unsaved work is periodically persisted and restored on next launch.
- **Dark, minimal UI** — Flat/Zinc-inspired theme built with Tailwind CSS.

## Screenshot

![QuickText](screenshots/quicktext.jpg)

## Tech Stack

| Layer      | Technology                                  |
| ---------- | ------------------------------------------- |
| Backend    | Go 1.23 + [Wails v2](https://wails.io)     |
| Frontend   | Vue 3 (Composition API) + Vite + Tailwind  |
| Editor     | Monaco Editor + xterm.js                    |

## Prerequisites

- Go 1.23+
- Node.js + npm
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Linux build dependencies (GTK, webkit2gtk) — see the [Wails Linux prerequisites](https://wails.io/docs/gettingstarted/installation#linux).

## Development

Run the app in live-reload mode:

```bash
wails dev
```

This starts a Vite dev server with hot reload. A browser dev server is also available at `http://localhost:34115`.

## Build

Create a redistributable production build:

```bash
wails build
```

The output binary is placed in `build/bin/`. For a Debian package:

```bash
wails build -platform linux/amd64
```

## Project Structure

```
.
├── main.go            # Wails app entrypoint & window config
├── app.go             # Go backend: file I/O, terminal, autosave, safe close
├── frontend/          # Vue 3 + Vite frontend
│   └── src/
│       ├── App.vue    # Layout, tabs, sidebar, close handling
│       └── components/
├── build/             # Build assets & packaging (icons, installers)
└── wails.json         # Project configuration
```

## License

Distributed under the [GNU General Public License v3.0](LICENSE).

Copyright © 2026 PerfLite.
